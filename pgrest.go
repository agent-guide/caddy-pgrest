package caddypgrest

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"

	"github.com/jackc/pgx/v5/pgxpool"
)

func init() {
	caddy.RegisterModule(PGRestHandler{})
	httpcaddyfile.RegisterHandlerDirective("pgrest_graphql", parsePGRestGraphql)
}

type PGRestHandler struct {
	// "postgres://user:password@localhost:5432/dbname"
	PgUrl     string `json:"pgurl,omitempty"`
	TableName string `json:"table_name,omitempty"`

	pool   *pgxpool.Pool
	logger *zap.Logger
}

type GraphQLRequest struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
}

// CaddyModule returns the Caddy module information.
func (PGRestHandler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.pgrest",
		New: func() caddy.Module { return new(PGRestHandler) },
	}
}

// Provision implements caddy.Provisioner.
func (m *PGRestHandler) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger()

	if m.PgUrl == "" {
		return fmt.Errorf("pgurl is empty")
	}
	pool, err := pgxpool.New(ctx, m.PgUrl)
	if err != nil {
		return fmt.Errorf("pgxpool new error: %v", err)
	}
	m.pool = pool
	return nil
}

// Validate implements caddy.Validator.
func (m *PGRestHandler) Validate() error {
	if m.pool == nil {
		return fmt.Errorf("no pool")
	}
	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (m PGRestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		m.logger.Info("pgrest ServerHttp",
			zap.String("ContentType", contentType),
		)
		return next.ServeHTTP(w, r)
	}

	var reqData GraphQLRequest
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&reqData)
	if err != nil {
		return fmt.Errorf("invalid pgrest reqeust: %v", err)
	}
	m.logger.Debug("pgrest ServerHttp",
		zap.String("GraphQLRequestQuery", reqData.Query),
		zap.String("GraphQLRequestOperationName", reqData.OperationName),
	)

	var result []byte
	err = m.pool.QueryRow(
		r.Context(),
		`select graphql.resolve($1, $2)`,
		reqData.Query,
		reqData.Variables,
	).Scan(&result)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		response := map[string]string{"message": "Data received successfully"}
		json.NewEncoder(w).Encode(response)
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(result)
	return nil
}

// Interface guards
var (
	_ caddy.Provisioner           = (*PGRestHandler)(nil)
	_ caddy.Validator             = (*PGRestHandler)(nil)
	_ caddyhttp.MiddlewareHandler = (*PGRestHandler)(nil)
)

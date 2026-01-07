package caddypgrest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/jackc/pgx/v5/pgxpool"
)

func init() {
	caddy.RegisterModule(Middleware{})
	httpcaddyfile.RegisterHandlerDirective("pgrest", parseCaddyfile)
}

type Middleware struct {
	// "postgres://user:password@localhost:5432/dbname"
	PgUrl string `json:"pgurl,omitempty"`

	pool *pgxpool.Pool
}

type GraphQLRequest struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
}

// CaddyModule returns the Caddy module information.
func (Middleware) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.pgrest",
		New: func() caddy.Module { return new(Middleware) },
	}
}

// Provision implements caddy.Provisioner.
func (m *Middleware) Provision(ctx caddy.Context) error {
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
func (m *Middleware) Validate() error {
	if m.pool == nil {
		return fmt.Errorf("no pool")
	}
	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (m Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	contentType := r.Header.Get("Content-Type")
	if contentType == "application/json" {
		var reqData GraphQLRequest
		defer r.Body.Close()
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&reqData)
		if err != nil {
			return fmt.Errorf("invalid pgrest reqeust: %v", err)
		}

		var result []byte
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err = m.pool.QueryRow(
			ctx,
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
	} else {
		fmt.Println("not application/json")
	}
	return next.ServeHTTP(w, r)
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (m *Middleware) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	d.Next() // consume directive name

	// require an argument
	if !d.NextArg() {
		return d.ArgErr()
	}

	// store the argument
	m.PgUrl = d.Val()
	return nil
}

// parseCaddyfile unmarshals tokens from h into a new Middleware.
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m Middleware
	err := m.UnmarshalCaddyfile(h.Dispenser)
	return m, err
}

// Interface guards
var (
	_ caddy.Provisioner           = (*Middleware)(nil)
	_ caddy.Validator             = (*Middleware)(nil)
	_ caddyhttp.MiddlewareHandler = (*Middleware)(nil)
	_ caddyfile.Unmarshaler       = (*Middleware)(nil)
)

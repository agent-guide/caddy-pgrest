package caddypgrest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

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

// validateGraphQLQuery checks if the query only accesses the allowed table
func (m *PGRestHandler) validateGraphQLQuery(query string) error {
	if m.TableName == "" {
		return fmt.Errorf("table_name is not configured")
	}
	tableName := "*"
	if m.TableName != "*" {
		tableName = m.TableName + "Collection"
	}

	// Remove GraphQL comments (# ...)
	cleanQuery := query
	re := regexp.MustCompile(`#.*$`)
	cleanQuery = re.ReplaceAllString(cleanQuery, "")

	// Find the top-level selection set (the first { after query/mutation/subscription)
	// Extract content between the first { and matching }
	topLevelContent := extractTopLevelSelection(cleanQuery)
	if topLevelContent == "" {
		return fmt.Errorf("invalid GraphQL query: cannot find top-level selection set")
	}

	if tableName == "*" {
		return nil
	}

	// Extract top-level field names from the selection set
	topLevelFields := extractTopLevelFields(topLevelContent)

	// Check if any top-level field is not the allowed table
	for _, field := range topLevelFields {
		if field != tableName {
			return fmt.Errorf("permission denied: access to table '%s' is not allowed", field)
		}
	}

	return nil
}

// extractTopLevelSelection extracts the content of the first top-level selection set
func extractTopLevelSelection(query string) string {
	// Remove query/mutation/subscription keyword if present
	cleaned := regexp.MustCompile(`^\s*(query|mutation|subscription)\s*\w*\s*\{`).ReplaceAllString(query, "{")

	// Find the first opening brace
	firstBrace := strings.Index(cleaned, "{")
	if firstBrace == -1 {
		return ""
	}

	// Find the matching closing brace
	braceCount := 0
	start := firstBrace
	for i := start; i < len(cleaned); i++ {
		if cleaned[i] == '{' {
			braceCount++
		} else if cleaned[i] == '}' {
			braceCount--
			if braceCount == 0 {
				return cleaned[start+1 : i]
			}
		}
	}

	return ""
}

// extractTopLevelFields extracts field names from a selection set
// This only extracts fields at the current level, not nested ones
func extractTopLevelFields(selection string) []string {
	var fields []string

	// Split by braces to isolate top-level content
	parts := strings.Split(selection, "{")
	if len(parts) == 0 {
		return fields
	}

	// The first part contains top-level field names
	topLevelPart := parts[0]

	// Extract field names (alphanumeric words followed by arguments or braces)
	fieldPattern := regexp.MustCompile(`(\w+)\s*(?:\(|\{)`)
	matches := fieldPattern.FindAllStringSubmatch(topLevelPart, -1)
	for _, match := range matches {
		if len(match) > 1 {
			fields = append(fields, match[1])
		}
	}

	return fields
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
func (m *PGRestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
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

	// Validate that the query only accesses the allowed table
	if err := m.validateGraphQLQuery(reqData.Query); err != nil {
		m.logger.Warn("pgrest permission denied",
			zap.String("table", m.TableName),
			zap.Error(err),
		)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		response := map[string]string{"error": err.Error()}
		json.NewEncoder(w).Encode(response)
		return fmt.Errorf("permission denied: %v", err)
	}

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

package caddypgrest

import (
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// parseCaddyfile parses the pgrest_graphql directive.
// Syntax options:
//
//	Block: pgrest_graphql {
//	    db_url <connection_string>
//	    table_name <name>
//	}
func parsePGRestGraphql(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var pgURL, tableName string

	if !h.Next() { // consume directive name
		return nil, h.Err("expected directive name")
	}

	// Check for arguments (should be none)
	if h.NextArg() {
		return nil, h.ArgErr()
	}

	// If there's a block, parse subdirectives
	for h.NextBlock(0) {
		switch h.Val() {
		case "db_url":
			if !h.NextArg() {
				return nil, h.ArgErr()
			}
			pgURL = h.Val()
		case "table_name":
			if !h.NextArg() {
				return nil, h.ArgErr()
			}
			tableName = h.Val()
		default:
			return nil, h.Errf("unrecognized subdirective: %s", h.Val())
		}
	}

	if pgURL == "" {
		return nil, h.ArgErr()
	}
	if tableName == "" {
		return nil, h.ArgErr()
	}

	// Create handler with config
	m := &PGRestHandler{
		PgUrl:     pgURL,
		TableName: tableName,
	}

	return m, nil
}

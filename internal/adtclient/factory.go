// internal/adtclient/factory.go
package adtclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Hochfrequenz/adtler/adt"
	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"

	"github.com/hochfrequenz/ai-abap-code-review-service/internal/btp"
)

// FORK: "HF_S4" is the BTP destination name. "100" is the SAP client number.
// apply-config rewrites both from config.yml (examples.destination_name and
// examples.sap_client) when you fork this repo.
const (
	destinationName = "HF_S4"
	sapClientNumber = "100"
)

// NewFromBTPEnv builds an adtler Client that routes through the BTP
// Connectivity service's SOCKS5 proxy to the on-premise SAP system.
// This is the single place in the service that bridges internal/btp and adtler.
func NewFromBTPEnv(ctx context.Context, env btp.Env) (adt.Client, error) {
	// NewTokenFetcher takes an optional *http.Client (nil = 10s default).
	// Fetch takes (ctx, tokenBaseURL, clientID, clientSecret); env.Dest.URL
	// is the XSUAA token base URL from the destination service binding.
	fetcher := btp.NewTokenFetcher(nil)

	destToken, err := fetcher.Fetch(ctx, env.Dest.URL, env.Dest.ClientID, env.Dest.ClientSecret)
	if err != nil {
		return nil, fmt.Errorf("destination token: %w", err)
	}

	// LookupDestination takes (ctx, httpClient, cred, bearer, name).
	// Passing nil for httpClient uses a 10s default.
	dest, err := btp.LookupDestination(ctx, nil, env.Dest, destToken, destinationName)
	if err != nil {
		return nil, fmt.Errorf("lookup destination %q: %w", destinationName, err)
	}

	// ConnTokenProvider takes *http.Request so cancellation propagates
	// into the token fetch.
	provider := btp.ConnTokenProvider(func(req *http.Request) (string, error) {
		return fetcher.Fetch(req.Context(), env.Conn.URL, env.Conn.ClientID, env.Conn.ClientSecret)
	})

	transport, err := btp.NewOnPremiseTransport(env.Conn, provider)
	if err != nil {
		return nil, fmt.Errorf("on-premise transport: %w", err)
	}

	cfg := sapmcpconfig.SAPSystem{
		Host:     dest.URL,
		User:     dest.User,
		Password: dest.Password,
		Client:   sapClientNumber,
	}
	return adt.NewClientWithTransport(cfg, transport), nil
}

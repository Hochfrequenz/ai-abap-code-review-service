// web/xs_app_test.go
package web_test

import (
	"encoding/json"
	"os"
	"testing"
)

type xsAppRoute struct {
	Source           string `json:"source"`
	Destination      string `json:"destination"`
	AuthenticationTy string `json:"authenticationType"`
	CSRFProtection   *bool  `json:"csrfProtection"`
}

type xsApp struct {
	Routes []xsAppRoute `json:"routes"`
}

// TestAPIRouteHasCSRFProtectionEnabled guards against a regression of the
// approuter CSRF check on /api/*. The approuter enables CSRF protection by
// default for every route; only an explicit "csrfProtection": false turns
// it off. This test fails if that override reappears on the /api route.
func TestAPIRouteHasCSRFProtectionEnabled(t *testing.T) {
	raw, err := os.ReadFile("xs-app.json")
	if err != nil {
		t.Fatalf("read xs-app.json: %v", err)
	}
	var cfg xsApp
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse xs-app.json: %v", err)
	}

	found := false
	for _, route := range cfg.Routes {
		if route.Source != `^/api/(.*)$` {
			continue
		}
		found = true
		if route.CSRFProtection != nil && !*route.CSRFProtection {
			t.Error(`route "^/api/(.*)$" must not set "csrfProtection": false — the approuter's CSRF check must stay enabled on /api/*`)
		}
	}
	if !found {
		t.Fatal(`xs-app.json has no route matching source "^/api/(.*)$"`)
	}
}

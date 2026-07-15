package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
	"go.uber.org/zap"
)

func TestNewServerUsesExplicitConfiguration(t *testing.T) {
	server, err := app.NewServer(app.ServerConfig{
		Addr: ":0",
		Sessions: admin.NewSessionManager(admin.SessionConfig{
			TTL: time.Hour, SameSite: http.SameSiteLaxMode, Secure: false,
		}),
		CSRF:     func(next http.Handler) http.Handler { return next },
		Frontend: app.FrontendConfig{Mode: app.FrontendDisabled},
	}, nil, zap.NewNop())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if server == nil {
		t.Fatal("nil server")
	}
}

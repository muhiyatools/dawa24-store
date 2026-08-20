package main

import (
	"context"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// adminGatewaySettings reads the credentials an operator saved in
// /admin/settings. It lives here, not in platform/, because platform packages
// must not import business modules.
type adminGatewaySettings struct {
	svc *platformadmin.Service
}

func newAdminGatewaySettings(svc *platformadmin.Service) *adminGatewaySettings {
	return &adminGatewaySettings{svc: svc}
}

func (a *adminGatewaySettings) GatewaySettings(ctx context.Context) (gateway.Settings, error) {
	if a == nil || a.svc == nil {
		return gateway.Settings{}, nil
	}
	// database.AsSystem is justified: Gateway settings are platform-wide,
	// not scoped to any single organization tenant.
	sysCtx := database.AsSystem(ctx)
	gw, err := a.svc.GetGatewaySettings(sysCtx)
	if err != nil || gw == nil {
		return gateway.Settings{}, err
	}

	return gateway.Settings{
		BaseURL:    gw.EndpointURL,
		VirtualKey: gw.APIKey,
		ClientApp:  "Dawa24Store",
		Enabled:    gw.IsActive,
	}, nil
}

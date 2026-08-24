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

	// 1. Check Gateway Settings
	gw, _ := a.svc.GetGatewaySettings(sysCtx)
	if gw != nil && gw.APIKey != "" {
		return gateway.Settings{
			BaseURL:    gw.EndpointURL,
			VirtualKey: gw.APIKey,
			ClientApp:  "Dawa24Store",
			Enabled:    gw.IsActive,
		}, nil
	}

	// 2. Fallback to AI Settings if configured there
	ai, _ := a.svc.GetAISettings(sysCtx)
	if ai != nil && ai.APIKey != "" {
		return gateway.Settings{
			BaseURL:    ai.EndpointURL,
			VirtualKey: ai.APIKey,
			ClientApp:  "Dawa24Store",
			Enabled:    ai.IsActive,
		}, nil
	}

	if gw != nil {
		return gateway.Settings{
			BaseURL:    gw.EndpointURL,
			VirtualKey: gw.APIKey,
			ClientApp:  "Dawa24Store",
			Enabled:    gw.IsActive,
		}, nil
	}

	return gateway.Settings{}, nil
}

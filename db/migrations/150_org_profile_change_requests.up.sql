-- 150_org_profile_change_requests.up.sql
--
-- Adds organization profile change requests workflow.
-- Instead of instant profile overwrites, changes submitted by organizations
-- are recorded as change requests for platform administrator review and approval.

CREATE TABLE IF NOT EXISTS org.organization_change_requests (
    id BIGSERIAL PRIMARY KEY,
    public_id UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES org.organizations(id) ON DELETE CASCADE,
    requested_by BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    current_values JSONB NOT NULL DEFAULT '{}'::jsonb,
    proposed_values JSONB NOT NULL DEFAULT '{}'::jsonb,
    reviewed_by BIGINT REFERENCES identity.users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    admin_notes TEXT NOT NULL DEFAULT '',
    rejection_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_org_change_requests_org ON org.organization_change_requests (organization_id);
CREATE INDEX IF NOT EXISTS idx_org_change_requests_status ON org.organization_change_requests (status);
CREATE INDEX IF NOT EXISTS idx_org_change_requests_created ON org.organization_change_requests (created_at DESC);

-- Enable and force Row-Level Security
ALTER TABLE org.organization_change_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE org.organization_change_requests FORCE ROW LEVEL SECURITY;

-- Tenant isolation policy: tenants can only see and insert requests for their own organization.
-- Platform admin or system processes (app.is_system()) can see and manage all requests.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE schemaname = 'org' 
          AND tablename = 'organization_change_requests' 
          AND policyname = 'tenant_isolation'
    ) THEN
        CREATE POLICY tenant_isolation ON org.organization_change_requests
            FOR ALL TO dawa24_app
            USING (app.is_system() OR organization_id = app.current_org_id())
            WITH CHECK (app.is_system() OR organization_id = app.current_org_id());
    END IF;
END $$;

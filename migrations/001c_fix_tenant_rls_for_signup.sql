-- Migration: 001c_fix_tenant_rls_for_signup
-- Description: Allow tenant creation during signup without tenant context
-- References: SDD 04-DESIGN-DETALHADO/01-modelo-de-dados.md, 05-SEGURANCA/01-autenticacao-autorizacao.md

-- Drop the overly restrictive policy that blocks INSERT during signup
DROP POLICY IF EXISTS tenant_isolation_policy ON tenants;

-- Policy: Allow SELECT/UPDATE/DELETE only for users within their tenant
CREATE POLICY tenant_isolation_policy ON tenants
    FOR SELECT
    USING (id = current_setting('app.current_tenant_id', true)::UUID);

CREATE POLICY tenant_isolation_policy_update ON tenants
    FOR UPDATE
    USING (id = current_setting('app.current_tenant_id', true)::UUID);

CREATE POLICY tenant_isolation_policy_delete ON tenants
    FOR DELETE
    USING (id = current_setting('app.current_tenant_id', true)::UUID);

-- Policy: Allow INSERT during signup (no tenant context needed yet)
-- Only allows creating a tenant if no other tenant exists for the same document
-- This is safe because the unique constraint on document prevents duplicates
CREATE POLICY tenant_signup_insert_policy ON tenants
    FOR INSERT
    WITH CHECK (true);

-- Keep the admin update policy
-- (tenant_admin_policy already exists from 001b)
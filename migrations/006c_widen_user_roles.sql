-- Migration 006c: Widen users.role and invites.role to the full RBAC role set
-- Domain: Multitenancy / User management
-- Execute AFTER 006b_invites.sql, BEFORE 007_billing_fase_a.sql
-- 001_tenants_users.sql only allowed ADMIN/ENGINEER/VIEWER; code now uses
-- SUPER_ADMIN/OWNER/ADMIN/MANAGER/ENGINEER/VIEWER (see domain.UserRole).

DO $$
DECLARE
    con_name TEXT;
BEGIN
    SELECT conname INTO con_name
    FROM pg_constraint
    WHERE conrelid = 'users'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%role%IN%';

    IF con_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE users DROP CONSTRAINT %I', con_name);
    END IF;
END $$;

ALTER TABLE users
    ADD CONSTRAINT users_role_check
        CHECK (role IN ('SUPER_ADMIN','OWNER','ADMIN','MANAGER','ENGINEER','VIEWER'));

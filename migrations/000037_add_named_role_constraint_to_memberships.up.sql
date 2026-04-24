-- Add a named CHECK constraint for the role column on group_memberships.
--
-- Migration 000031 introduced the role column with an anonymous inline CHECK.
-- A named constraint makes the allowed values self-documenting in pg_constraint
-- and produces a clearer error message ("chk_group_memberships_role" instead of
-- the system-generated "group_memberships_role_check") when an invalid value is
-- rejected.
--
-- Allowed values correspond 1-to-1 with the Go constants in domain/entities.go:
--   'member' → MembershipRoleMember      (regular participant)
--   'owner'  → MembershipRoleCreateOwner (group creator / current owner)
ALTER TABLE group_memberships
    ADD CONSTRAINT chk_group_memberships_role
    CHECK (role IN ('member', 'owner'));

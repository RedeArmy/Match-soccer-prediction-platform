-- Migration 000144: seed KYC document retention system parameter.
--
-- Adds the kyc.doc_retention_years parameter that controls how long KYC
-- document metadata is kept after a user account is soft-deleted.
--
-- The default of 5 years is a conservative best-practice for platforms that
-- perform AML-flagged transaction monitoring (UAF AuditActionAMLFlagged).
-- Guatemala does not impose a specific KYC document retention period on
-- betting quinielas, but 5 years aligns with the general SIB/UAF guidance
-- for entities subject to the Ley Contra el Lavado de Dinero u Otros Activos.
--
-- is_runtime=FALSE: changing the retention period requires consideration and
-- cannot be hot-reloaded — a worker restart is required to pick up the new
-- value, and the next weekly purge job will apply it.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    ('kyc.doc_retention_years', '5', '5', 'int', 'kyc', FALSE,
     'Number of years to retain KYC document metadata after a user account is soft-deleted. '
     'Default: 5 years. Changing this value requires a worker restart and takes effect on '
     'the next weekly kyc.document_purge job run (Sunday 03:00 UTC).')
ON CONFLICT (key) DO NOTHING;

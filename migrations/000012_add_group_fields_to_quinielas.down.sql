ALTER TABLE quinielas
    DROP CONSTRAINT IF EXISTS quinielas_name_key,
    DROP CONSTRAINT IF EXISTS quinielas_invite_code_key,
    DROP COLUMN IF EXISTS max_members,
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS entry_fee,
    DROP COLUMN IF EXISTS invite_code;

DELETE FROM system_params WHERE key IN (
    'tournament.general_entry_fee_cents',
    'tournament.round_entry_fee_cents'
);

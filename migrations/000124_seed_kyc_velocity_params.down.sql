DELETE FROM system_params WHERE key IN (
    'kyc.tier1_deposit_velocity_cents',
    'kyc.tier2_deposit_velocity_cents',
    'kyc.tier1_withdrawal_velocity_cents',
    'kyc.tier2_withdrawal_velocity_cents'
);

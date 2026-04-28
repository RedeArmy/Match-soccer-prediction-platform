UPDATE system_params
   SET category = 'messaging'
 WHERE key IN ('dlq.sample_size', 'dlq.replay_default_limit');

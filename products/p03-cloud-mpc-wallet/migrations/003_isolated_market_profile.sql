ALTER TABLE wallet_mpc.wallet DROP CONSTRAINT wallet_profile_check;
ALTER TABLE wallet_mpc.wallet ADD CONSTRAINT wallet_profile_check
  CHECK (profile = 'sepolia-native-test-v1' OR profile ~ '^isolated-market-8283-v1:[0-9a-f]{64}$');

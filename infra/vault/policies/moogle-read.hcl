path "secret/data/moogle/*" {
  capabilities = ["read"]
}

path "secret/metadata/moogle/*" {
  capabilities = ["list", "read"]
}

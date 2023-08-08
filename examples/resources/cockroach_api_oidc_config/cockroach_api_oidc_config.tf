resource "cockroach_api_oidc_config" "example" {
  issuer   = "https://accounts.google.com"
  audience = "test_audience"
  jwks     = "{\"keys\":[{\"alg\":\"RS256\",\"e\":\"AQAB\",\"kid\":\"test_kid1\",\"kty\":\"RSA\",\"n\":\"09lq1lCEuteonwDJOhGTDak11ThplZuC9JEWQNdBnBSQwlkJQIE7A7nTBO0xTibcsh2HwYkC-N_Gs1jP4iwN3dRqnu5FwG2ct5mY8KLwJiHzToFC0MKenSFQCy0FviNtOnpiObcUlDvR2NDeNtMl_6SPzcQEt7GUTBBYZgoAxPmOgevki6ZNO6Y86xFqx3y6v8EPwW010AiC60r4AHGCTBhYF4uqmq5JH2UU4dDh9Udc-9LZxlSqPwJvnKDG2GjcnD8TsU3wjfEM_nRmx3dnXsrZUXYfNGtdv5dlHywf5AhkJmTavqcsJkgrNA-PNBghFMcCR816_kCIkCYWLWC5vQ\"}]}"
  claim    = "sub"
  identity_map = [
    {
      token_identity = "token_identity"
      cc_identity    = "cc_identity"
      is_regex       = false
    },
    {
      token_identity = "(.*)"
      cc_identity    = "\\1@example.com"
      is_regex       = true
    },
  ]
}

package admission

import (
	"errors"
	"fmt"
	"testing"
)

const (
	validRSA1024Key = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQCLFEu0y7Gl2sG0TCHKBKntvzf5Dszt/SWm5GJXIriGCAKdaOKqmeA/AfECqkE9q/omX8rkr+4RdLVRm2ybkQHYinf7IUmmWcjifnB2STDVeHBkgggYY0MC0Dom5pYMfklUZSWiH1XulFSZd7XsCKcxIloWxxljunsv2BUhUaguSw==`
	validRSA2048Key = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDk6xzVo5JU9FYzE6HNZeMq9mqNfMOr6rBX7QZ317ZL1TMSdIvvQzvuJmn0ZvkrwpT5vLsSYQex9gz/62xr6Unb7i7rXUsPhq4TDDucWwGis7GJ78lFvt4kPW81kqPJiiSh3uIUA/enVLBrXZbGLd1AfHd+rENrhjq6mFyd42CbNunHPiQAgMJKZ3mRb/llzo5fKZeR1KbETwsjVbPkD5fW026HlIsT8QJ49ya7xuZCgF9iPcL9EUTpQkK60r4iNAnzodlS5YsErLck+P+Jw1xEJ+hw0BTBgXtFQznTVFMrV7E408o9+UY/t7Sb6wE1HUEDbIdaKyPUT158FNugVeP7`
	validRSA4096Key = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQCSskDVXwtW3fVkpbZkCeWj/aBr8lE+NyHbgVsAbmylb1MzrjWAJ1ynFSRCFk1fBql1zrmNJPeT6d5SkfvqExRiaC7KZ+oucvM9lkjyVwREQEF1d5iBQr3268C+S4HKgKxFYaJQwMYw7bYnE7np6kHwTOTX5sOC4imFWKR40X385yItbkmL35ZgIJQB8/W+TCEU4wEND5Kf3m85d1pIVCTqn3NTf3s3BtezK1AJQtzqJDVGALqrmgf9+fGM8yheMRKKwVi378hkREI4oOcWppLV20IDKE4OFm6ZW+U414zcq75WkebRgThK4Y0EepqUxebd1A3KoTeEMaJeHGUmhq6YOjKkAg9PyTKNBsDjwwOCzIuoFbmEOq7H9e3fE670unuM/O92NOwPK7XTedNryNs7QMe+UPzO3HP9nGYziy+rBCgnGs2QJjYya8ReKKB34G9VtBRn7vRd0lXliVFjUcQKhpClJdVENVbRH3MJrsE+iWOf46u8kI9xrSAdo0BX6z0x/ujIH8cI1FFZZxToSWP/VrqIr0wtMOwiQ7j5VeEFN1S4ACYm/dzzG01j0Xr0bdJ/2PqSASf4S1HEI9KEzLWYIHtjhHjLwS8narweW0fMjtUu2tRlwxGoS5aZP4JYIjHd9DWlkczswDkh2OmMpaNPXnr3f2BITxgea/SPkoUdYw==`

	validECDSA256Key = `ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBFDpHqvvX0D9iLccVi53JthsQ4xSJicRfl0oAPCdTgyGAQG1RXI435o4wG+bAD1zOUMjdfd2iVykwRZ6R7+yOaM=`
	validECDSA384Key = `ecdsa-sha2-nistp384 AAAAE2VjZHNhLXNoYTItbmlzdHAzODQAAAAIbmlzdHAzODQAAABhBFm76lHa5m5O1nOyQTyG6JoSktq6/l3tHj80nuxfxV7xJwV47guLwFsK5vGpnhFcC3cmBl2GO/deis6EalCaaWoi/sGEnJrFCLUEMxRojX/pHNYPaU4R2DZnj0Y2w/y03A==`
	validECDSA521Key = `ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAG2GjQxcul7nGRsuEtbTfKxYskKaYMaGPLwptG+1PQRPDGgTEqiGUToDYXhui6DyGKIZz/3i6iYyeEVgz6+wc+eqAENZMrA7qi0t4NXk6ky56PJeLLHb9Ry0Isdi6idoIZnKrv184Afc4hY4EeiK8Q+oseQwjLYg+0gwb+q9zPr4IXuZA==`

	validDSA1024Key = `ssh-dss AAAAB3NzaC1kc3MAAACBAP1/U4EddRIpUt9KnC7s5Of2EbdSPO9EAMMeP4C2USZpRV1AIlH7WT2NWPq/xfW6MPbLm1Vs14E7gB00b/JmYLdrmVClpJ+f6AR7ECLCT7up1/63xhv4O1fnxqimFQ8E+4P208UewwI1VBNaFpEy9nXzrith1yrv8iIDGZ3RSAHHAAAAFQCXYFCPFSMLzLKSuYKi64QL8Fgc9QAAAIEA9+GghdabPd7LvKtcNrhXuXmUr7v6OuqC+VdMCz0HgmdRWVeOutRZT+ZxBxCBgLRJFnEj6EwoFhO3zwkyjMim4TwWeotUfI0o4KOuHiuzpnWRbqN/C/ohNWLx+2J6ASQ7zKTxvqhRkImog9/hWuWfBpKLZl6Ae1UlZAFMO/7PSSoAAACAU5qGNxrBT4VDW1bN1m6szPH4PRlqNSPHNG/1Xs3LrJyGRXxnl218IYyrfAb+lIIEZEcUFGGWyRJOLQhmWv68zBupKv1JJaVAQ4JTMPPmmPwGus01eSGd9NjAS6Qtl9FGMLrLFk4IRFuenHWOas1PzDlOXybUnaXtQpNcKEJgMik=`
)

func TestValidatePublicKeys(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		err  error
	}{
		{
			name: "valid keys",
			keys: []string{
				//RSA
				validRSA1024Key,
				validRSA2048Key,
				validRSA4096Key,

				// ECDSA
				validECDSA256Key,
				validECDSA384Key,
				validECDSA521Key,

				// DSA
				validDSA1024Key,
			},
		},
		{
			name: "invalid key",
			keys: []string{"some invalid key"},
			err:  errors.New(`invalid public key "some invalid key": ssh: no key found`),
		},
		{
			name: "one of many is invalid",
			keys: []string{
				validRSA1024Key,
				"some invalid key",
			},
			err: errors.New(`invalid public key "some invalid key": ssh: no key found`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePublicKeys(test.keys)
			if fmt.Sprint(err) != fmt.Sprint(test.err) {
				t.Errorf("Expected error to be\n%v\ninstead got\n%v", test.err, err)
			}
		})
	}
}

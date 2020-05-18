package paid

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyStreamAccess(t *testing.T) {
	noError := func(t *testing.T, err error) { assert.NoError(t, err) }
	type tokenMaker func() (string, error)
	type errChecker func(*testing.T, error)

	tests := []struct {
		name       string
		makeToken  tokenMaker
		checkError errChecker
	}{
		{
			name: "valid",
			makeToken: func() (string, error) {
				return CreateToken(testStreamID, testTxID, 120_000_000, ExpTenSecPer100MB)
			},
			checkError: noError,
		},
		{
			name: "expired",
			makeToken: func() (string, error) {
				expFunc := func(uint64) int64 { return 1 } //  Returns the 1st second of Unix epoch
				return CreateToken(testStreamID, testTxID, 120_000_000, expFunc)
			},
			checkError: func(t *testing.T, err error) { assert.Regexp(t, "token is expired by \\d+h\\d+m\\d+s", err) },
		},
		{
			name: "missigned",
			makeToken: func() (string, error) {
				otherPkey, _ := rsa.GenerateKey(rand.Reader, 2048)
				otherKM := &keyManager{privKey: otherPkey}
				return otherKM.createToken(testStreamID, testTxID, 120_000_000, ExpTenSecPer100MB)
			},
			checkError: func(t *testing.T, err error) { assert.EqualError(t, err, "crypto/rsa: verification error") },
		},
		{
			name: "wrong_stream",
			makeToken: func() (string, error) {
				return CreateToken("wrOngsTream", testTxID, 120_000_000, ExpTenSecPer100MB)
			},
			checkError: func(t *testing.T, err error) {
				assert.EqualError(t, err, "stream mismatch: requested bea4d30a1868a00e98297cfe8cdefc1be6c141b54bea3b7c95b34a66786c22ab4e9f35ae19aa453b3630e76afbd24fe2, token valid for wrOngsTream")
			},
		},
		{
			name: "wrong_token",
			makeToken: func() (string, error) {
				return "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzaWQiOiJpT1MtMTMtQWRvYmVYRC85Y2QyZTkzYmZjNzUyZGQ2NTYwZTQzNjIzZjM2ZDBjMzUwNGRiY2E2IiwidHhpZCI6Ijg2NzhiY2Y0NzAxNGZhMDA0ZjU3ZjQ2NTE2NjdjYWM2MmRmMzk5MDA2NjIyOTg2YjI1YTNmNWY2YTAwYTViNTgiLCJleHAiOjE1ODk5MDg3ODUsImlhdCI6MTU4OTgyNTI3OX0.igcb5DBB_XDvPqJRFIg3rurHfCds6UoPyV37oMvxnDsGG5WOK6VAcQJXZPkEH2LUxvebRsI9exLhUJUlqqwYxduiBlM9VHrj-7SmC5FNOrkZ4iZ5gfOS-Tyc5b4GwslACC6JBtlx_tL4qOeCxXDyEfEpnLvZVy_SlKdiwBWASZA0E56NLDVW74wQYkEWy17mK_stXcySopmjkbJQ8hDhw93NtcGy4MYVgPJviq-c4YHm8boDKzxbp_CEK5hyUh5jxxC1F2yIMZiHWBS6Dy1YrhFW5jDUh-Y-09B6abAVxCm052Ut6RHPaM9rxRgLJ9QZXdb8iFq6NG32XIWffVn-Jz", nil
			},
			checkError: func(t *testing.T, err error) {
				assert.EqualError(t, err, "crypto/rsa: verification error")
			},
		},
		{
			name: "partial_token",
			makeToken: func() (string, error) {
				return "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9", nil
			},
			checkError: func(t *testing.T, err error) {
				assert.EqualError(t, err, "token contains an invalid number of segments")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := tt.makeToken()
			require.NoError(t, err)

			err = VerifyStreamAccess(testStreamID, token)
			tt.checkError(t, err)
		})
	}
}

func BenchmarkParseToken(b *testing.B) {
	token, err := CreateToken(testStreamID, testTxID, 100_000_000, ExpTenSecPer100MB)
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		if err := VerifyStreamAccess(testStreamID, token); err != nil {
			b.Fatal(err)
		}
	}
}

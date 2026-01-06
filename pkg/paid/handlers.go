package paid

import (
	"net/http"
)

// HandlePublicKeyRequest delivers marshaled pubkey to remote agents for media token verification
func HandlePublicKeyRequest(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write(km.PublicKeyBytes())
}

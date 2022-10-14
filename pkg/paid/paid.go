package paid

// Key files should be generated as follows:
// $ openssl genrsa -out privateKey.pem 2048

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/OdyseeTeam/player-server/pkg/logger"

	"github.com/golang-jwt/jwt/v4"
)

var Logger = logger.GetLogger()

// Expfunc is a function type intended for CreateToken.
// Should take stream size in bytes and return validity as Unix time
type Expfunc func(uint64) int64

// StreamToken contains stream ID and transaction id of a stream that has been purchased.
// StreamID can be any string uniquely identifying a stream, has to be consistent
// for both API server and player server
type StreamToken struct {
	StreamID string `json:"sid"`
	TxID     string `json:"txid"`
	jwt.StandardClaims
}

type keyManager struct {
	privKey         *rsa.PrivateKey
	pubKeyMarshaled []byte
}

var km *keyManager

// InitPrivateKey loads a private key from `[]bytes` for later token signing and derived pubkey distribution.
func InitPrivateKey(rawKey []byte) error {
	km = &keyManager{}
	err := km.loadFromBytes(rawKey)
	if err != nil {
		return err
	}
	return nil
}

// CreateToken takes stream ID, purchase transaction id and stream size to generate a JWS.
// In addition it accepts expiry function that takes streamSize and returns token expiry date as Unix time
func CreateToken(streamID string, txid string, streamSize uint64, expfunc Expfunc) (string, error) {
	return km.createToken(streamID, txid, streamSize, expfunc)
}

// GeneratePrivateKey generates an in-memory private key
func GeneratePrivateKey() error {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	k := &keyManager{privKey: privKey}
	k.pubKeyMarshaled, err = k.marshalPublicKey()
	if err != nil {
		return err
	}
	Logger.Infof("generated an in-memory private key")

	km = k
	err = InitPubKey(k.PublicKeyBytes())
	if err != nil {
		return err
	}
	return nil
}

func (k *keyManager) createToken(streamID string, txid string, streamSize uint64, expfunc Expfunc) (string, error) {
	if k.privKey == nil {
		return "", fmt.Errorf("cannot create a token, private key is not initialized (call InitPrivateKey)")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, &StreamToken{
		streamID,
		txid,
		jwt.StandardClaims{
			ExpiresAt: expfunc(streamSize),
			IssuedAt:  time.Now().UTC().Unix(),
		},
	})
	Logger.Debugf("created a token %v / %v", token.Header, token.Claims)
	return token.SignedString(k.privKey)
}

func (k *keyManager) loadFromBytes(b []byte) error {
	block, _ := pem.Decode(b)
	if block == nil {
		return errors.New("no PEM blob found")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	k.privKey = key
	Logger.Infof("loaded a private RSA key (%v bytes)", key.Size())

	k.pubKeyMarshaled, err = k.marshalPublicKey()
	if err != nil {
		return err
	}

	return nil
}

func (k *keyManager) PublicKeyBytes() []byte {
	return k.pubKeyMarshaled
}

func (k *keyManager) PublicKeyManager() *pubKeyManager {
	return &pubKeyManager{key: &k.privKey.PublicKey}
}

func (k *keyManager) marshalPublicKey() ([]byte, error) {
	pubKey, err := x509.MarshalPKIXPublicKey(&k.privKey.PublicKey)
	if err != nil {
		return nil, err
	}

	pubBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pubKey,
	})

	return pubBytes, nil
}

// ExpTenSecPer100MB returns expiry date as calculated by 10 seconds times the stream size in MB
func ExpTenSecPer100MB(streamSize uint64) int64 {
	return time.Now().UTC().Add(time.Duration(streamSize/1024^2*10) * time.Second).Unix()
}

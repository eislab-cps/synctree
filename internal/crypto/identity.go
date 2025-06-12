package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
)

type Identity struct {
	prv *ecdsa.PrivateKey
	id  string
}

func CreateIdentity() (*Identity, error) {
	identity := &Identity{}

	prv, err := ecdsa.GenerateKey(btcec.S256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	if prv == nil {
		return nil, errors.New("Invalid private key")
	}

	identity.prv = prv
	identity.id = GenerateHashFromString(identity.PublicKeyAsHex()).String()

	return identity, nil
}

func (identity *Identity) PrivateKey() *ecdsa.PrivateKey {
	return identity.prv
}

func (identity *Identity) PrivateKeyAsHex() string {
	n := identity.prv.Params().BitSize / 8
	binaryDump := make([]byte, n)
	if identity.prv.D.BitLen()/8 >= n {
		binaryDump = identity.prv.D.Bytes()
	} else {
		i := len(binaryDump)
		for _, d := range identity.prv.D.Bits() {
			for j := 0; j < wordBytes && i > 0; j++ {
				i--
				binaryDump[i] = byte(d)
				d >>= 8
			}
		}
	}

	return hex.EncodeToString(binaryDump)
}

func (identity *Identity) ID() string {
	return identity.id
}

func GenerateID(hexEncodedPrv string) (string, error) {
	identity, err := CreateIdentityFromString(hexEncodedPrv)
	if err != nil {
		return "", err
	}

	return identity.ID(), nil
}

func CreateIdentityFromString(hexEncodedPrv string) (*Identity, error) {
	identity := &Identity{}
	decodedPrv, err := hex.DecodeString(hexEncodedPrv)

	if err != nil {
		return nil, err
	}

	prv := new(ecdsa.PrivateKey)
	prv.PublicKey.Curve = btcec.S256()

	if 8*len(decodedPrv) != prv.Params().BitSize {
		return nil, fmt.Errorf("Invalid private key length, should be %d bits", prv.Params().BitSize)
	}

	prv.D = new(big.Int).SetBytes(decodedPrv)
	if prv.D.Cmp(secp256k1N) >= 0 {
		return nil, fmt.Errorf("Invalid private key")
	}
	if prv.D.Sign() <= 0 {
		return nil, fmt.Errorf("Invalid private key")
	}

	prv.PublicKey.X, prv.PublicKey.Y = prv.PublicKey.Curve.ScalarBaseMult(decodedPrv)
	if prv.PublicKey.X == nil {
		return nil, errors.New("Invalid private key")
	}

	identity.prv = prv
	identity.id = GenerateHashFromString(identity.PublicKeyAsHex()).String()

	return identity, nil
}

func (identity *Identity) PublicKey() []byte {
	return elliptic.Marshal(btcec.S256(), identity.prv.PublicKey.X, identity.prv.PublicKey.Y)
}

func (identity *Identity) PublicKeyAsHex() string {
	return hex.EncodeToString(identity.PublicKey())
}

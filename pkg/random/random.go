package random

import (
	"github.com/eislab-cps/synctree/pkg/security/crypto"

	"github.com/google/uuid"
)

func GenerateRandomID() string {
	uuid := uuid.New()
	crypto := crypto.CreateCrypto()
	return crypto.GenerateHash(uuid.String())
}

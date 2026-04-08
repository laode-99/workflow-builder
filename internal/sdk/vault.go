package sdk

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/pkg/crypto"
	"gorm.io/gorm"
)

// GetCredential fetches and decrypts a credential from the vault.
// integration is the type key (e.g. "retell_ai", "google_sheets").
func GetCredential(db *gorm.DB, businessID uuid.UUID, integration string, encKey []byte) (string, error) {
	var cred model.Credential
	// Try business-specific first
	err := db.Where("business_id = ? AND integration = ?", businessID, integration).First(&cred).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Fallback: search for a GLOBAL credential for this integration
			err = db.Where("is_global = true AND integration = ?", integration).First(&cred).Error
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return "", fmt.Errorf("no business or global credential found for: %s", integration)
				}
				return "", err
			}
		} else {
			return "", err
		}
	}

	plaintext, err := crypto.Decrypt(encKey, cred.DataEnc)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %v", err)
	}

	return string(plaintext), nil
}

// GetCredentialByID fetches a specific credential by its ID.
func GetCredentialByID(db *gorm.DB, credID uuid.UUID, encKey []byte) (string, error) {
	var cred model.Credential
	if err := db.First(&cred, "id = ?", credID).Error; err != nil {
		return "", fmt.Errorf("credential %s not found", credID)
	}

	plaintext, err := crypto.Decrypt(encKey, cred.DataEnc)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %v", err)
	}

	return string(plaintext), nil
}

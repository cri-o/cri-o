//
// Copyright 2021 The Sigstore Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rekord

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"golang.org/x/sync/errgroup"

	"github.com/sigstore/rekor/pkg/generated/models"
	"github.com/sigstore/rekor/pkg/log"
	"github.com/sigstore/rekor/pkg/pki"
	"github.com/sigstore/rekor/pkg/types"
	"github.com/sigstore/rekor/pkg/types/rekord"
	"github.com/sigstore/rekor/pkg/util"
)

const (
	APIVERSION = "0.0.1"
)

func init() {
	if err := rekord.VersionMap.SetEntryFactory(APIVERSION, NewEntry); err != nil {
		log.Logger.Panic(err)
	}
}

type V001Entry struct {
	RekordObj models.RekordV001Schema
}

func (v V001Entry) APIVersion() string {
	return APIVERSION
}

func NewEntry() types.EntryImpl {
	return &V001Entry{}
}

func (v V001Entry) IndexKeys() ([]string, error) {
	var result []string

	af, err := pki.NewArtifactFactory(pki.Format(*v.RekordObj.Signature.Format))
	if err != nil {
		return nil, err
	}
	keyObj, err := af.NewPublicKey(bytes.NewReader(*v.RekordObj.Signature.PublicKey.Content))
	if err != nil {
		return nil, err
	}

	key, err := keyObj.CanonicalValue()
	if err != nil {
		log.Logger.Error(err)
	} else {
		keyHash := sha256.Sum256(key)
		result = append(result, strings.ToLower(hex.EncodeToString(keyHash[:])))
	}

	result = append(result, keyObj.Subjects()...)

	if v.RekordObj.Data.Hash != nil {
		hashKey := strings.ToLower(fmt.Sprintf("%s:%s", *v.RekordObj.Data.Hash.Algorithm, *v.RekordObj.Data.Hash.Value))
		result = append(result, hashKey)
	}

	return result, nil
}

func (v *V001Entry) Unmarshal(pe models.ProposedEntry) error {
	rekord, ok := pe.(*models.Rekord)
	if !ok {
		return errors.New("cannot unmarshal non Rekord v0.0.1 type")
	}

	if err := types.DecodeEntry(rekord.Spec, &v.RekordObj); err != nil {
		return err
	}

	// field validation
	if err := v.RekordObj.Validate(strfmt.Default); err != nil {
		return err
	}

	// cross field validation
	return v.validate()

}

func (v *V001Entry) fetchExternalEntities(ctx context.Context) (pki.PublicKey, pki.Signature, error) {
	g, ctx := errgroup.WithContext(ctx)

	af, err := pki.NewArtifactFactory(pki.Format(*v.RekordObj.Signature.Format))
	if err != nil {
		return nil, nil, err
	}

	hashR, hashW := io.Pipe()
	sigR, sigW := io.Pipe()
	defer hashR.Close()
	defer sigR.Close()

	closePipesOnError := types.PipeCloser(hashR, hashW, sigR, sigW)

	oldSHA := ""
	if v.RekordObj.Data.Hash != nil && v.RekordObj.Data.Hash.Value != nil {
		oldSHA = swag.StringValue(v.RekordObj.Data.Hash.Value)
	}

	g.Go(func() error {
		defer hashW.Close()
		defer sigW.Close()

		dataReadCloser := bytes.NewReader(v.RekordObj.Data.Content)

		/* #nosec G110 */
		if _, err := io.Copy(io.MultiWriter(hashW, sigW), dataReadCloser); err != nil {
			return closePipesOnError(err)
		}
		return nil
	})

	hashResult := make(chan string)

	g.Go(func() error {
		defer close(hashResult)
		hasher := sha256.New()

		if _, err := io.Copy(hasher, hashR); err != nil {
			return closePipesOnError(err)
		}

		computedSHA := hex.EncodeToString(hasher.Sum(nil))
		if oldSHA != "" && computedSHA != oldSHA {
			return closePipesOnError(types.ValidationError(fmt.Errorf("SHA mismatch: %s != %s", computedSHA, oldSHA)))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case hashResult <- computedSHA:
			return nil
		}
	})

	sigResult := make(chan pki.Signature)

	g.Go(func() error {
		defer close(sigResult)

		sigReadCloser := bytes.NewReader(*v.RekordObj.Signature.Content)

		signature, err := af.NewSignature(sigReadCloser)
		if err != nil {
			return closePipesOnError(types.ValidationError(err))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case sigResult <- signature:
			return nil
		}
	})

	keyResult := make(chan pki.PublicKey)

	g.Go(func() error {
		defer close(keyResult)

		keyReadCloser := bytes.NewReader(*v.RekordObj.Signature.PublicKey.Content)

		key, err := af.NewPublicKey(keyReadCloser)
		if err != nil {
			return closePipesOnError(types.ValidationError(err))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case keyResult <- key:
			return nil
		}
	})

	var (
		keyObj pki.PublicKey
		sigObj pki.Signature
	)
	g.Go(func() error {
		keyObj, sigObj = <-keyResult, <-sigResult

		if keyObj == nil || sigObj == nil {
			return closePipesOnError(errors.New("failed to read signature or public key"))
		}

		var err error
		if err = sigObj.Verify(sigR, keyObj); err != nil {
			return closePipesOnError(types.ValidationError(err))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})

	computedSHA := <-hashResult

	if err := g.Wait(); err != nil {
		return nil, nil, err
	}

	// if we get here, all goroutines succeeded without error
	if oldSHA == "" {
		v.RekordObj.Data.Hash = &models.RekordV001SchemaDataHash{}
		v.RekordObj.Data.Hash.Algorithm = swag.String(models.RekordV001SchemaDataHashAlgorithmSha256)
		v.RekordObj.Data.Hash.Value = swag.String(computedSHA)
	}

	return keyObj, sigObj, nil
}

func (v *V001Entry) Canonicalize(ctx context.Context) ([]byte, error) {
	keyObj, sigObj, err := v.fetchExternalEntities(ctx)
	if err != nil {
		return nil, err
	}

	canonicalEntry := models.RekordV001Schema{}

	// need to canonicalize signature & key content
	canonicalEntry.Signature = &models.RekordV001SchemaSignature{}
	// signature URL (if known) is not set deliberately
	canonicalEntry.Signature.Format = v.RekordObj.Signature.Format

	var sigContent []byte
	sigContent, err = sigObj.CanonicalValue()
	if err != nil {
		return nil, err
	}
	canonicalEntry.Signature.Content = (*strfmt.Base64)(&sigContent)

	var pubKeyContent []byte
	canonicalEntry.Signature.PublicKey = &models.RekordV001SchemaSignaturePublicKey{}
	pubKeyContent, err = keyObj.CanonicalValue()
	if err != nil {
		return nil, err
	}
	canonicalEntry.Signature.PublicKey.Content = (*strfmt.Base64)(&pubKeyContent)

	canonicalEntry.Data = &models.RekordV001SchemaData{}
	canonicalEntry.Data.Hash = v.RekordObj.Data.Hash
	// data content is not set deliberately

	// wrap in valid object with kind and apiVersion set
	rekordObj := models.Rekord{}
	rekordObj.APIVersion = swag.String(APIVERSION)
	rekordObj.Spec = &canonicalEntry

	v.RekordObj = canonicalEntry

	bytes, err := json.Marshal(&rekordObj)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

// validate performs cross-field validation for fields in object
func (v V001Entry) validate() error {
	sig := v.RekordObj.Signature
	if v.RekordObj.Signature == nil {
		return errors.New("missing signature")
	}
	if sig.Content == nil || len(*sig.Content) == 0 {
		return errors.New("'content' must be specified for signature")
	}

	key := sig.PublicKey
	if key == nil {
		return errors.New("missing public key")
	}
	if key.Content == nil || len(*key.Content) == 0 {
		return errors.New("'content' must be specified for publicKey")
	}

	data := v.RekordObj.Data
	if data == nil {
		return errors.New("missing data")
	}

	hash := data.Hash
	if hash != nil {
		if !govalidator.IsHash(swag.StringValue(hash.Value), swag.StringValue(hash.Algorithm)) {
			return errors.New("invalid value for hash")
		}
	} else if len(data.Content) == 0 {
		return errors.New("'content' must be specified for data")
	}

	return nil
}

func (v V001Entry) CreateFromArtifactProperties(ctx context.Context, props types.ArtifactProperties) (models.ProposedEntry, error) {
	returnVal := models.Rekord{}
	re := V001Entry{}

	// we will need artifact, public-key, signature
	re.RekordObj.Data = &models.RekordV001SchemaData{}

	var err error
	artifactBytes := props.ArtifactBytes
	if artifactBytes == nil {
		var artifactReader io.ReadCloser
		if props.ArtifactPath == nil {
			return nil, errors.New("path to artifact file must be specified")
		}
		if props.ArtifactPath.IsAbs() {
			artifactReader, err = util.FileOrURLReadCloser(ctx, props.ArtifactPath.String(), nil)
			if err != nil {
				return nil, fmt.Errorf("error reading artifact file: %w", err)
			}
		} else {
			artifactReader, err = os.Open(filepath.Clean(props.ArtifactPath.Path))
			if err != nil {
				return nil, fmt.Errorf("error opening artifact file: %w", err)
			}
		}
		artifactBytes, err = io.ReadAll(artifactReader)
		if err != nil {
			return nil, fmt.Errorf("error reading artifact file: %w", err)
		}
	}
	re.RekordObj.Data.Content = strfmt.Base64(artifactBytes)

	re.RekordObj.Signature = &models.RekordV001SchemaSignature{}
	switch props.PKIFormat {
	case "pgp":
		re.RekordObj.Signature.Format = swag.String(models.RekordV001SchemaSignatureFormatPgp)
	case "minisign":
		re.RekordObj.Signature.Format = swag.String(models.RekordV001SchemaSignatureFormatMinisign)
	case "x509":
		re.RekordObj.Signature.Format = swag.String(models.RekordV001SchemaSignatureFormatX509)
	case "ssh":
		re.RekordObj.Signature.Format = swag.String(models.RekordV001SchemaSignatureFormatSSH)
	}
	sigBytes := props.SignatureBytes
	if sigBytes == nil {
		if props.SignaturePath == nil {
			return nil, errors.New("a detached signature must be provided")
		}
		sigBytes, err = os.ReadFile(filepath.Clean(props.SignaturePath.Path))
		if err != nil {
			return nil, fmt.Errorf("error reading signature file: %w", err)
		}
		re.RekordObj.Signature.Content = (*strfmt.Base64)(&sigBytes)
	} else {
		re.RekordObj.Signature.Content = (*strfmt.Base64)(&sigBytes)
	}

	re.RekordObj.Signature.PublicKey = &models.RekordV001SchemaSignaturePublicKey{}
	publicKeyBytes := props.PublicKeyBytes
	if len(publicKeyBytes) == 0 {
		if len(props.PublicKeyPaths) != 1 {
			return nil, errors.New("only one public key must be provided to verify detached signature")
		}
		keyBytes, err := os.ReadFile(filepath.Clean(props.PublicKeyPaths[0].Path))
		if err != nil {
			return nil, fmt.Errorf("error reading public key file: %w", err)
		}
		publicKeyBytes = append(publicKeyBytes, keyBytes)
	} else if len(publicKeyBytes) != 1 {
		return nil, errors.New("only one public key must be provided")
	}

	re.RekordObj.Signature.PublicKey.Content = (*strfmt.Base64)(&publicKeyBytes[0])

	if err := re.validate(); err != nil {
		return nil, err
	}

	if _, _, err := re.fetchExternalEntities(ctx); err != nil {
		return nil, fmt.Errorf("error retrieving external entities: %v", err)
	}

	returnVal.APIVersion = swag.String(re.APIVersion())
	returnVal.Spec = re.RekordObj

	return &returnVal, nil
}

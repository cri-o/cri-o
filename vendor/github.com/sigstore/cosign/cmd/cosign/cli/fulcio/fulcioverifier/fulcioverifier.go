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

package fulcioverifier

import (
	"context"
	"fmt"
	"os"

	"github.com/sigstore/cosign/cmd/cosign/cli/fulcio"
	"github.com/sigstore/cosign/cmd/cosign/cli/fulcio/fulcioverifier/ctl"
	"github.com/sigstore/cosign/cmd/cosign/cli/options"
)

func NewSigner(ctx context.Context, ko options.KeyOpts) (*fulcio.Signer, error) {
	fs, err := fulcio.NewSigner(ctx, ko)
	if err != nil {
		return nil, err
	}

	// verify the sct
	if err := ctl.VerifySCT(ctx, fs.Cert, fs.Chain, fs.SCT); err != nil {
		return nil, fmt.Errorf("verifying SCT: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Successfully verified SCT...")

	return fs, nil
}

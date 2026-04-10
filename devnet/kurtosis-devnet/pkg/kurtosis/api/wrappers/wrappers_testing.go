//go:build testonly
// +build testonly

package wrappers

import (
	"errors"

	"github.com/datachainlab/optimism-devnet/devnet/kurtosis-devnet/pkg/kurtosis/api/interfaces"
)

func GetDefaultKurtosisContext() (interfaces.KurtosisContextInterface, error) {
	return nil, errors.New("attempting to use local Kurtosis context in testonly mode")
}

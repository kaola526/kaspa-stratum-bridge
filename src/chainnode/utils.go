package chainnode

import (
	"errors"
	"regexp"
	"strings"

	"github.com/kaspanet/kaspad/util"
)

var walletRegex = regexp.MustCompile("kaspa:[a-z0-9]+")

func CleanWallet(chain_type string, in string) (string, error) {
	if chain_type == ChainTypeKaspa {
		_, err := util.DecodeAddress(in, util.Bech32PrefixKaspa)
		if err == nil {
			return in, nil // good to go
		}
		if !strings.HasPrefix(in, "kaspa:") {
			return CleanWallet(chain_type, "kaspa:"+in)
		}

		// has kaspa: prefix but other weirdness somewhere
		if walletRegex.MatchString(in) {
			return in[0:67], nil
		}
		return "", errors.New("unable to coerce wallet to valid kaspa address")
	} else {
		return "", nil
	}
}

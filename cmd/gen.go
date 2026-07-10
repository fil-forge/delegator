package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/fil-forge/libforge/commands/claim"
	"github.com/fil-forge/libforge/commands/space/egress"
	"github.com/fil-forge/libforge/identity"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/multikey"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/delegation"
	"github.com/spf13/cobra"
)

var GenCmd = &cobra.Command{
	Use:          "gen",
	Aliases:      []string{"g"},
	Short:        "Generate a UCAN delegation",
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	RunE:         mkDelegation,
}

var (
	// Gen command flags
	issuerPrivateKeyFile string
	issuerDidWebKey      string
	audienceDidKey       string
	command              string
	expiration           int64
)

func init() {
	GenCmd.Flags().StringVarP(&issuerPrivateKeyFile, "issuer-private-key-file", "f", "", "Path to PEM encoded Ed25519 private key of delegation issuer")
	cobra.CheckErr(GenCmd.MarkFlagRequired("issuer-private-key-file"))

	GenCmd.Flags().StringVarP(&issuerDidWebKey, "issuer-did-web", "i", "", "Optional did:web: of issuer, when provided wraps did:key: of delegation issuer")

	GenCmd.Flags().StringVarP(&audienceDidKey, "audience-did-key", "a", "", "did:key of delegation audience")
	cobra.CheckErr(GenCmd.MarkFlagRequired("audience-did-key"))

	GenCmd.Flags().StringVarP(&command, "command", "c", "", "command issuer will authorize to audience")
	cobra.CheckErr(GenCmd.MarkFlagRequired("command"))

	GenCmd.Flags().Int64VarP(&expiration, "expiration", "e", 0, "expiration time in UTC seconds since Unix epoch")
}

func mkDelegation(cmd *cobra.Command, _ []string) error {
	signer, err := parseIssuerKey(issuerPrivateKeyFile)
	if err != nil {
		return fmt.Errorf("parsing issuer private key from file %s: %w", issuerPrivateKeyFile, err)
	}

	issuer := multikey.KeyIssuer(signer)
	if issuerDidWebKey != "" {
		issuerDidWeb, err := did.Parse(issuerDidWebKey)
		if err != nil {
			return fmt.Errorf("parsing issuer did web key (%s): %w", issuerDidWebKey, err)
		}
		if issuerDidWeb.Method() != "web" {
			return fmt.Errorf("issuer did:web: must start with 'did:web:' prefix")
		}
		issuer = multikey.NewIssuer(issuerDidWeb, signer)
	}

	audience, err := did.Parse(audienceDidKey)
	if err != nil {
		return fmt.Errorf("parsing audience did key: %w", err)
	}

	var opts []delegation.Option
	if expiration > 0 {
		if time.Now().Unix() > expiration {
			return fmt.Errorf("provided expiration time %d is in the past", expiration)
		}
		opts = append(opts, delegation.WithExpiration(ucan.UnixTimestamp(expiration)))
	} else {
		opts = append(opts, delegation.WithNoExpiration())
	}

	// Subject must be the issuer's own DID — the issuer is delegating
	// authority over its own resources to the audience. Using `audience`
	// here produces a delegation whose subject is the delegator (not the
	// indexer/etracker), which fails downstream chain validation with
	// "delegation subject is X not Y" when piri later uses this as a
	// proof for invoking against the indexing/egress-tracker service.
	subject := issuer.DID()

	var d ucan.Delegation
	if command == claim.Cache.Command.String() {
		d, err = claim.Cache.Delegate(issuer, audience, subject, opts...)
		if err != nil {
			return fmt.Errorf("creating delegation: %w", err)
		}
	} else if command == egress.Track.Command.String() {
		d, err = egress.Track.Delegate(issuer, audience, subject, opts...)
		if err != nil {
			return fmt.Errorf("creating delegation: %w", err)
		}
	} else {
		return fmt.Errorf("unknown command: %s", command)
	}

	out, err := delegation.Encode(d)
	if err != nil {
		return fmt.Errorf("formatting delegation: %w", err)
	}
	fmt.Println(string(out))
	return nil

}

// parseIssuerKey attempts to read and parse the private key from the
// provided path.
func parseIssuerKey(path string) (multikey.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	return identity.DecodeSignerFromPEM(data)
}

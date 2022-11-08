// Driver for DRKey experiments

package drkey

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/drkey"
	"github.com/scionproto/scion/pkg/drkey/specific"
	"github.com/scionproto/scion/pkg/private/serrors"
	cppb "github.com/scionproto/scion/pkg/proto/control_plane"
	dkpb "github.com/scionproto/scion/pkg/proto/drkey"
	"github.com/scionproto/scion/pkg/scrypto/cppki"
	"github.com/scionproto/scion/pkg/snet"
)

func RunDemo(daemonAddr string, serverMode bool, serverAddr, clientAddr snet.UDPAddr) {
	ctx, cancelF := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelF()

	// meta describes the key that both client and server derive
	meta := drkey.HostHostMeta{
		ProtoId: drkey.SCMP,
		// Validity timestamp; both sides need to use a validity time stamp in the same epoch.
		// Usually this is coordinated by means of a timestamp in the message.
		Validity: time.Now(),
		// SrcIA is the AS on the "fast side" of the DRKey derivation;
		// the server side in this example.
		SrcIA: serverAddr.IA,
		// DstIA is the AS on the "slow side" of the DRKey derivation;
		// the client side in this example.
		DstIA:   clientAddr.IA,
		SrcHost: serverAddr.Host.IP.String(),
		DstHost: clientAddr.Host.IP.String(),
	}

	daemon, err := daemon.NewService(daemonAddr).Connect(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error dialing SCION Daemon:", err)
		return
	}

	if serverMode {
		// Server: get the Secret Value (SV) for the protocol and derive all
		// subsequent keys in-process
		server := Server{daemon}
		// Fetch the Secret Value (SV); in a real application, this is only done at
		// startup and refreshed for each epoch.
		sv, err := server.fetchSV(ctx, drkey.SecretValueMeta{
			Validity: meta.Validity,
			ProtoId:  meta.ProtoId,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error fetching secret value:", err)
			return
		}
		t0 := time.Now()
		serverKey, err := server.DeriveHostHostKey(sv, meta)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error deriving key:", err)
			return
		}
		durationServer := time.Since(t0)

		fmt.Printf(
			"Server,\thost key = %s\tduration = %s\n",
			hex.EncodeToString(serverKey.Key[:]),
			durationServer,
		)
	} else {
		// Client: fetch key from daemon
		// The daemon will in turn obtain the key from the local CS
		// The CS will fetch the Lvl1 key from the CS in the SrcIA (the server's AS)
		// and derive the Host key based on this.
		client := Client{daemon}
		t0 := time.Now()
		clientKey, err := client.FetchHostHostKey(ctx, meta)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error fetching key:", err)
			return
		}
		durationClient := time.Since(t0)

		fmt.Printf(
			"Client,\thost key = %s\tduration = %s\n",
			hex.EncodeToString(clientKey.Key[:]),
			durationClient,
		)
	}
}

type Client struct {
	daemon daemon.Connector
}

func (c Client) FetchHostHostKey(
	ctx context.Context, meta drkey.HostHostMeta) (drkey.HostHostKey, error) {

	// get L2 key: (slow path)
	return c.daemon.DRKeyGetHostHostKey(ctx, meta)
}

type Server struct {
	daemon daemon.Connector
}

func (s Server) DeriveHostHostKey(
	sv drkey.SecretValue,
	meta drkey.HostHostMeta,
) (drkey.HostHostKey, error) {

	var deriver specific.Deriver
	lvl1, err := deriver.DeriveLevel1(meta.DstIA, sv.Key)
	if err != nil {
		return drkey.HostHostKey{}, serrors.WrapStr("deriving level 1 key", err)
	}
	asHost, err := deriver.DeriveHostAS(meta.SrcHost, lvl1)
	if err != nil {
		return drkey.HostHostKey{}, serrors.WrapStr("deriving host-AS key", err)
	}
	hosthost, err := deriver.DeriveHostHost(meta.DstHost, asHost)
	if err != nil {
		return drkey.HostHostKey{}, serrors.WrapStr("deriving host-host key", err)
	}
	return drkey.HostHostKey{
		ProtoId: sv.ProtoId,
		Epoch:   sv.Epoch,
		SrcIA:   meta.SrcIA,
		DstIA:   meta.DstIA,
		SrcHost: meta.SrcHost,
		DstHost: meta.DstHost,
		Key:     hosthost,
	}, nil
}

// fetchSV obtains the Secret Value (SV) for the selected protocol/epoch.
// From this SV, all keys for this protocol/epoch can be derived locally.
// The IP address of the server must be explicitly allowed to abtain this SV
// from the the control server.
func (s Server) fetchSV(
	ctx context.Context,
	meta drkey.SecretValueMeta,
) (drkey.SecretValue, error) {

	// Obtain CS address from scion daemon
	svcs, err := s.daemon.SVCInfo(ctx, nil)
	if err != nil {
		return drkey.SecretValue{}, serrors.WrapStr("obtaining control service address", err)
	}
	cs := svcs[addr.SvcCS]
	if len(cs) == 0 {
		return drkey.SecretValue{}, serrors.New("no control service address found")
	}

	// Contact CS directly for SV
	conn, err := grpc.DialContext(ctx, cs[0], grpc.WithInsecure())
	if err != nil {
		return drkey.SecretValue{}, serrors.WrapStr("dialing control service", err)
	}
	defer conn.Close()
	client := cppb.NewDRKeyIntraServiceClient(conn)

	rep, err := client.DRKeySecretValue(ctx, &cppb.DRKeySecretValueRequest{
		ValTime:    timestamppb.New(meta.Validity),
		ProtocolId: dkpb.Protocol(meta.ProtoId),
	})
	if err != nil {
		return drkey.SecretValue{}, serrors.WrapStr("requesting drkey secret value", err)
	}

	key, err := getSecretFromReply(meta.ProtoId, rep)
	if err != nil {
		return drkey.SecretValue{}, serrors.WrapStr("validating drkey secret value reply", err)
	}

	return key, nil
}

func getSecretFromReply(
	proto drkey.Protocol,
	rep *cppb.DRKeySecretValueResponse,
) (drkey.SecretValue, error) {

	if err := rep.EpochBegin.CheckValid(); err != nil {
		return drkey.SecretValue{}, err
	}
	if err := rep.EpochEnd.CheckValid(); err != nil {
		return drkey.SecretValue{}, err
	}
	epoch := drkey.Epoch{
		Validity: cppki.Validity{
			NotBefore: rep.EpochBegin.AsTime(),
			NotAfter:  rep.EpochEnd.AsTime(),
		},
	}
	returningKey := drkey.SecretValue{
		ProtoId: proto,
		Epoch:   epoch,
	}
	copy(returningKey.Key[:], rep.Key)
	return returningKey, nil
}

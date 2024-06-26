package cheqd_interchaintest

import (
	"context"
	"encoding/json"
	"testing"

	sdjwttypes "github.com/nymlab/cheqd-interchaintest/types"
	interchaintest "github.com/strangelove-ventures/interchaintest/v7"
	"github.com/stretchr/testify/require"
)

func TestJunoStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	t.Parallel()

	ctx, cancelFn := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancelFn()
	})

	// create a single chain instance with x validators
	validatorsCount, fullnodeCount := 1, 1

	ic, juno, _, _ := CreateJunoChain(
		t,
		ctx,
		validatorsCount,
		fullnodeCount,
	)
	require.NotNil(t, ic)
	require.NotNil(t, juno)

	users := interchaintest.GetAndFundTestUsers(t, ctx, t.Name(), userFunds, juno)
	user := users[0]

	// ===================================
	// juno user upload and instantiate sdjwt contract
	// ===================================
	codeId, err := juno.StoreContract(
		ctx,
		user.KeyName(),
		contractPath,
	)
	require.NoError(t, err, "code store err")

	routeReqs := make([]sdjwttypes.RouteRequirement, 0)
	routeReq := sdjwttypes.RouteRequirement{
		RouteId: 1,
		Requirements: sdjwttypes.RouteVerificationRequirements{
			PresentationRequest: []byte("[]"),
			VerificationSource: sdjwttypes.VerificationSource{
				DataOrLocation: jwk,
			},
		},
	}

	initRegistrations := make([]sdjwttypes.InitRegistration, 0)
	var initMsg sdjwttypes.InstantiateMsg

	initMsg = sdjwttypes.InstantiateMsg{
		InitRegistrations: append(initRegistrations, sdjwttypes.InitRegistration{
			AppAdmin:   sdjwttypes.TestAppAddr1,
			AppAddress: sdjwttypes.TestAppAddr1,
			Routes:     append(routeReqs, routeReq),
		}),
		MaxPresentationLen: 30000,
	}

	initMsgBytes, err := json.Marshal(initMsg)

	junoNode := juno.FullNodes[0]

	contractAddr, err := junoNode.InstantiateContract(
		ctx,
		user.KeyName(),
		codeId,
		string(initMsgBytes),
		true,
		"--label",
		"avida-sdjwt",
		"--gas",
		"2000000",
	)
	require.NoError(t, err, "instantiate err")

	// ==========================================================================
	// Register a route with cheqd as trust registry and ensure it was registered
	// ==========================================================================

	registerMsg := sdjwttypes.ExecuteMsg{
		Register: &sdjwttypes.Register{
			AppAddr:       sdjwttypes.TestAppAddr2,
			RouteCriteria: append(routeReqs, routeReq)},
	}

	registerMsgBytes, err := json.Marshal(registerMsg)

	_, err = juno.ExecuteContract(
		ctx,
		user.KeyName(),
		contractAddr,
		string(registerMsgBytes),
	)
	require.NoError(t, err, "exec err")

	query, err := json.Marshal(
		sdjwttypes.QueryMsg{
			GetRoutes: &sdjwttypes.GetRoutes{
				AppAddr: sdjwttypes.TestAppAddr2,
			},
		},
	)

	var queryData sdjwttypes.GetRoutesRes
	err = junoNode.QueryContract(ctx, contractAddr, string(query), &queryData)
	require.NoError(t, err, "exec err")
	require.Len(t, queryData.Data, 1, "route length mismatch")
	require.Equal(t, uint64(0x1), queryData.Data[0], "RouteId mismatch")

	queryKey, err := json.Marshal(sdjwttypes.QueryMsg{
		GetRouteVerificationKey: &sdjwttypes.GetRouteVerificationKey{
			AppAddr: sdjwttypes.TestAppAddr2,
			RouteID: 1,
		},
	})

	var queryKeyData sdjwttypes.GetRouteVerificationKeyRes
	err = junoNode.QueryContract(ctx, contractAddr, string(queryKey), &queryKeyData)

	var originalJwk sdjwttypes.OkpJwk
	var returnedJwk sdjwttypes.OkpJwk

	err = json.Unmarshal(jwk, &originalJwk)
	err = json.Unmarshal([]byte(queryKeyData.Data), &returnedJwk)

	require.Equal(t, originalJwk, returnedJwk)

	t.Cleanup(func() {
		cancelFn()
	})
}

package chaincode

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	"github.com/hyperledger-firefly/firefly/chaincode-go/batchpin"
)

type SmartContract struct {
	contractapi.Contract
}

func (s *SmartContract) PinBatch(ctx contractapi.TransactionContextInterface, uuids, batchHash, payloadRef string, contexts []string) error {
	event, err := batchpin.BuildEvent(ctx, &batchpin.Args{
		UUIDs:      uuids,
		BatchHash:  batchHash,
		PayloadRef: payloadRef,
		Contexts:   contexts,
	})
	if err != nil {
		return err
	}
	bytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %s", err)
	}
	return ctx.GetStub().SetEvent("BatchPin", bytes)
}

func (s *SmartContract) NetworkAction(ctx contractapi.TransactionContextInterface, action, payload string) error {
	event, err := batchpin.BuildEvent(ctx, &batchpin.Args{})
	if err != nil {
		return err
	}
	event.Action = action
	event.PayloadRef = payload
	bytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %s", err)
	}
	return ctx.GetStub().SetEvent("BatchPin", bytes)
}

func (s *SmartContract) NetworkVersion() int {
	return 2
}

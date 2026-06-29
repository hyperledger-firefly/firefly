package chaincode

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	"github.com/hyperledger-firefly/firefly/custompin_sample/batchpin"
)

/**
 * Sample showing a simplistic way to support pinned off-chain messages via a custom method.
 * See FIR-17 documentation for how to leverage this functionality.
 */
type SmartContract struct {
	contractapi.Contract
}

func (s *SmartContract) MyCustomPin(ctx contractapi.TransactionContextInterface, data string) error {
	event, err := batchpin.BuildEventFromString(ctx, data)
	if err != nil {
		return err
	}
	bytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %s", err)
	}
	return ctx.GetStub().SetEvent("BatchPin", bytes)
}

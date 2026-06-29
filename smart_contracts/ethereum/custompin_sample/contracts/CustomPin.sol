// SPDX-License-Identifier: Apache-2.0

pragma solidity >=0.6.0 <0.9.0;

import "@hyperledger-firefly/firefly-contracts/contracts/IBatchPin.sol";

/**
 * Sample showing a simplistic way to support pinned off-chain messages via a custom method.
 * See FIR-17 documentation for how to leverage this functionality.
 */
contract CustomPin {
    IBatchPin firefly;

    event Hello();

    function setFireFlyAddress(address addr) external {
        firefly = IBatchPin(addr);
    }

    function sayHello(bytes calldata data) external {
        require(
            address(firefly) != address(0),
            "CustomPin: FireFly address has not been set"
        );
        emit Hello();
        firefly.pinBatchData(data);
    }
}

// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.7.0 <0.9.0;

interface ISignatureValidator {
    function isValidSignature(bytes32 _data, bytes memory _signature)
        external
        view
        returns (bytes4);
}

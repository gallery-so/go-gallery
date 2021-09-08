// SPDX-License-Identifier: MIT

pragma solidity ^0.8.0;

interface IRedeemable {
    function isRedeemed(uint256 _tokenID) external view returns (bool);

    function isRedeemedBy(uint256 tokenID, address redeemer)
        external
        view
        returns (bool);
}

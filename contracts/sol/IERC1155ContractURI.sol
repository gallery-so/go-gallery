// SPDX-License-Identifier: MIT

pragma solidity ^0.8.0;

/**
 * @title ERC-721 Non-Fungible Token Standard, optional metadata extension
 * @dev See https://eips.ethereum.org/EIPS/eip-721
 */
interface IERC1155ContractURI {
     * @dev Returns the Uniform Resource Identifier (URI) for the contract.
     */
    function contractURI() external view returns (string memory);
}

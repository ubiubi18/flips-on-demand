package main

import (
	"errors"
	"fmt"
	"strings"
)

// checkBuyTarget authenticates the payment destination against caller-supplied
// deployment pins. The request, marketplace snapshot, and prepared buy cannot
// supply either pin: they are the data being checked.
func checkBuyTarget(c *client, target, approvedContract, approvedCodeHash string) error {
	approvedContract = strings.ToLower(strings.TrimPrefix(approvedContract, "0x"))
	approvedCodeHash = strings.ToLower(strings.TrimPrefix(approvedCodeHash, "0x"))
	if !isHex(approvedContract, addressBytes) || !isHex(approvedCodeHash, 32) {
		return errors.New("buy requires independent -contract ADDRESS and -code-hash KECCAK256 pins")
	}
	if !strings.EqualFold(strings.TrimPrefix(target, "0x"), approvedContract) {
		return errors.New("buy target differs from the approved contract")
	}
	var stake struct {
		Hash *string `json:"Hash"`
	}
	if err := c.call("contract_getStake", []any{"0x" + approvedContract}, &stake); err != nil {
		return fmt.Errorf("read approved contract code hash: %w", err)
	}
	if stake.Hash == nil {
		return errors.New("approved contract has no deployed code hash")
	}
	actual := strings.ToLower(strings.TrimPrefix(*stake.Hash, "0x"))
	if !isHex(actual, 32) || actual != approvedCodeHash {
		return errors.New("approved contract code hash does not match the node")
	}
	return nil
}

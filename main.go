package main

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"github.com/btcsuite/golangcrypto/ripemd160"
	"github.com/go-errors/errors"
	"github.com/viacoin/viad/chaincfg"
	"github.com/viacoin/viad/chaincfg/chainhash"
	"github.com/viacoin/viad/txscript"
	"github.com/viacoin/viad/wire"
	btcutil "github.com/viacoin/viautil"
	"go/build"
	"time"
)

const (
	verify     = true
	secretSize = 32
	txVersion  = 2
)

//type Command interface {
//
//}

type initiateCmd struct {
	counterparty2Addr *btcutil.AddressPubKeyHash
	amount            btcutil.Amount
}

type participateCmd struct {
	counterparty1Addr *btcutil.AddressPubKeyHash
	amount            btcutil.Amount
}

type redeemCmd struct {
	contract   []byte
	contractTx *wire.MsgTx
}

type refundCmd struct {
	contract   []byte
	contractTx *wire.MsgTx
}

type extractSecretCmd struct {
	redemptionTx *wire.MsgTx
	secretHash   []byte
}

type Command struct {
	Command string
	Params  []string
}

type contractArgs struct {
	them       *btcutil.AddressPubKeyHash
	amount     btcutil.Amount
	locktime   int64
	secretHash []byte
}

func initiate(participantAddr string, amount float64) error {
	counterparty2Addr, err := btcutil.DecodeAddress(participantAddr, &chaincfg.MainNetParams)
	if err != nil {
		return fmt.Errorf("failed to decide the address from the participant: %s", err)
	}

	counterparty2AddrP2KH, ok := counterparty2Addr.(*btcutil.AddressPubKeyHash)
	if !ok {
		return errors.New("participant address is not P2KH")
	}

	amount2, err := btcutil.NewAmount(amount)
	if err != nil {
		return err
	}

	cmd := &initiateCmd{counterparty2Addr: counterparty2AddrP2KH, amount: amount2}
	return cmd.runCommand()
}

func (cmd *initiateCmd) runCommand() error {
	var secret [secretSize]byte
	_, err := rand.Read(secret[:])
	if err != nil {
		return err
	}

	secretHash := sha256Hash(secret[:])
	locktime := time.Now().Add(10 * time.Minute).Unix() // NEED TO CHANGE TO 48 HOURS

	x := &contractArgs{
		them:       cmd.counterparty2Addr,
		amount:     cmd.amount,
		locktime:   locktime,
		secretHash: secretHash,
	}
	return nil
}

// builtContract houses the details regarding a contract and the contract
// payment transaction, as well as the transaction to perform a refund.
type builtContract struct {
	contract       []byte
	contractP2SH   btcutil.Address
	contractTxHash *chainhash.Hash
	contractTx     *wire.MsgTx
	contractFee    btcutil.Amount
	refundTx       *wire.MsgTx
	refundFee      btcutil.Amount
}

func buildContract(args *contractArgs, refundAddr btcutil.Address) (*builtContract, error) {
	refundAddrHash, ok := refundAddr.(interface {
		Hash160() *[ripemd160.Size]byte
	})

	if !ok {
		return nil, errors.New("unable to create hash160 from change address")
	}
	contract, err  := atomicSwapContract(refundAddrHash.Hash160(), args.them.Hash160(), args.locktime, args.secretHash)
	if err != nil {
		return nil, err
	}
	contractP2SH, err := btcutil.NewAddressScriptHash(contract, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}
	contractP2SHPkScript, err := txscript.PayToAddrScript(contractP2SH)
	if err != nil {
		return nil, err
	}
	feePerKb := 0.01
	minFeePerKb := 0.02

	unsignedContract := wire.NewMsgTx(txVersion)
	unsignedContract.AddTxOut(wire.NewTxOut(int64(args.amount), contractP2SHPkScript))



	//contract, err := a

}

// atomicSwapContract returns an output script that may be redeemed by one of 2 signature scripts:
// <their sig> <their pubkey> <initiator secret> 1
// <my sig> <my pubkey> 0
func atomicSwapContract(pkhMe, pkhThem *[ripemd160.Size]byte, locktime int64, secretHash []byte) ([]byte, error) {
	builder := txscript.NewScriptBuilder()

	builder.AddOp(txscript.OP_IF) // if top of stack value is not False, execute. The top stack value is removed.
	{
		// require initiator's secret to be a known length that the redeeming party can audit.
		// this is used to prevent fraud attacks between 2 currencies that have different maximum data sizes
		builder.AddOp(txscript.OP_SIZE)        // pushes the string length of the top element of the stack (without popping it)
		builder.AddInt64(secretSize)           // pushes initiator secret length
		builder.AddOp(txscript.OP_EQUALVERIFY) // if inputs are equal, mark tx as valid

		// require initiator's secret to be known to redeem the output
		builder.AddOp(txscript.OP_SHA256)      // pushes the length of a SHA25 size
		builder.AddData(secretHash)            // push the data to the end of the script
		builder.AddOp(txscript.OP_EQUALVERIFY) // if inputs are equal, mark tx as valid

		// verify their signature is used to redeem the ouput
		// normally it ends with OP_EQUALVERIFY OP_CHECKSIG but
		// this has been moved outside of the branch to save a couple bytes
		builder.AddOp(txscript.OP_DUP)     // duplicates the stack of the top item
		builder.AddOp(txscript.OP_HASH160) // input has been hashed with SHA-256 and then with RIPEMD160 after
		builder.AddData(pkhThem[:])        // push the data to the end of the script
	}

	builder.AddOp(txscript.OP_ELSE) // refund path
	{
		// verify the locktime & drop if off the stack
		builder.AddInt64(locktime)                     // pushes locktime
		builder.AddOp(txscript.OP_CHECKLOCKTIMEVERIFY) // verify locktime
		builder.AddOp(txscript.OP_DROP)                // remove the top stack item (locktime)

		// verify our signature is being used to redeem the output
		// normally it ends with OP_EQUALVERIFY OP_CHECKSIG but
		// this has been moved outside of the branch to save a couple bytes
		builder.AddOp(txscript.OP_DUP)     // duplicates the stack of the top item
		builder.AddOp(txscript.OP_HASH160) // input has been hashed with SHA-256 and then with RIPEMD160 after
		builder.AddData(pkhMe[:])          // push the data to the end of the script

	}
	builder.AddOp(txscript.OP_ENDIF) // all blocks must end, or the transaction is invalid

	// returns 1 if the inputs are exactly equal, 0 otherwise.
	// mark transaction as invalid if top of stack is not true. The top stack value is removed.
	builder.AddOp(txscript.OP_EQUALVERIFY)

	// The entire transaction's outputs, inputs, and script are hashed.
	// The signature used by OP_CHECKSIG must be a valid signature for this hash
	// and public key. If it is, 1 is returned, 0 otherwise.
	builder.AddOp(txscript.OP_CHECKSIG)
	return builder.Script()
}

func sha256Hash(x []byte) []byte {
	hash := sha256.Sum256(x)
	return hash[:]
}

func main() {
	initiate("VtzJNYcoQUYGmKfZVo6VfCqVm8WDqdgz79", 2.09)
	//fmt.Println(a.amount)
}

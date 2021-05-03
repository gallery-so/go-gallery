package extern_services

import (
	"fmt"
	"testing"
	"context"
	"math/big"
	"github.com/stretchr/testify/assert"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//---------------------------------------------------
func Test__get_tx_logs(p_test *testing.T) {

	fmt.Println("TEST__OPENSEA ==============================================")
	
	ctx := context.Background()

	//--------------------
	// RUNTIME_SYS
	runtimeSys := &gf_core.Runtime_sys{
		Service_name_str: "gallery",

		// SENTRY - enable it for error reporting
		Errors_send_to_sentry_bool: true,
	}

	//--------------------
	ownerWalletAddressStr := "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"
	assetsForAccLst, gErr := OpenSeaPipelineAssetsForAcc(ownerWalletAddressStr, ctx, runtimeSys)
	if gErr != nil {
		p_test.Fail()
	}



	// assert.EqualValues(p_test, value_int, 445550000, "the decoded event Eth log value should be equal to 445550000")
	



}
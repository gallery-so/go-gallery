package main

import (
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/server"
)

func main() {

	config := runtime.ConfigLoad()
	portStr := config.Port

	// RUNTIME
	runtime, gErr := runtime.GetRuntime(config)
	if gErr != nil {
		panic(gErr.Error)
	}

	//-------------
	// SENTRY
	if runtime.RuntimeSys.Errors_send_to_sentry_bool {
		sentrySamplerate_f := 1.0

		// FINISH!! - create a complete list of handlers that we want to be traced.
		//            find a way to get this dynamically listed after the handlers are initialized,
		//            so that a list of handler paths doesnt have to get maintained in multiple places.
		sentryTransactionToTrace_map := map[string]bool{}

		err := gf_core.Error__init_sentry(runtime.Config.SentryEndpointStr,
			sentryTransactionToTrace_map,
			sentrySamplerate_f)
		if err != nil {
			panic(err)
		}

		defer sentry.Flush(2 * time.Second)
	}

	//-------------
	// SERVER_INIT
	server.Init(portStr, runtime)
}

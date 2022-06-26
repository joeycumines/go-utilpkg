package log

import (
	"errors"
	"github.com/sirupsen/logrus"
	"os"
)

func ExampleLogrus() {
	logger := func() Logger {
		logger := logrus.New()
		logger.SetOutput(os.Stdout)
		logger.SetFormatter(&logrus.TextFormatter{
			DisableColors:    true,
			DisableTimestamp: true,
		})
		return Logrus{Logger: logger}
	}()

	loggerA := logger.WithField(`a`, 1).
		WithField(`b`, 2).
		WithFields(map[string]any{
			`c`: 3,
			`d`: 4,
		}).
		WithError(errors.New(`5`))

	loggerB := logger.WithField(`a`, 6)

	logger.WithField(`e`, 7).
		Info(`one`)

	loggerA.WithField(`e`, 8).
		Info(`two`)

	loggerB.WithField(`e`, 9).
		Info(`three`)

	loggerA.Error(`four`)

	//output:
	//level=info msg=one e=7
	//level=info msg=two a=1 b=2 c=3 d=4 e=8 error=5
	//level=info msg=three a=6 e=9
	//level=error msg=four a=1 b=2 c=3 d=4 error=5
}

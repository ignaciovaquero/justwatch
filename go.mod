module github.com/igvaquero18/justwatch

go 1.14

replace github.com/igvaquero18/go-justwatch => ../go-justwatch

require (
	github.com/aws/aws-lambda-go v1.17.0
	github.com/igvaquero18/go-justwatch v0.0.0-20200711075952-d880a37eed33
	github.com/igvaquero18/telegram-notifier v0.0.0-20200709053438-7033b25bd928
	go.uber.org/zap v1.15.0
)

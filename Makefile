build: get_dep
	go build 

get_dep:
	go get github.com/go-telegram-bot-api/telegram-bot-api 
	go get gopkg.in/yaml.v2

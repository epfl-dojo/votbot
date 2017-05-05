package main

import (
  "fmt"
  "os"
	"log"
	"github.com/go-telegram-bot-api/telegram-bot-api"
  "net/http"
  "encoding/json"
  "io/ioutil"
)

var cardsJsonUrl = "https://api.trello.com/1/lists/58c1b0c5a2c599346fd571ac?fields=name&cards=open&card_fields=name,url"

type TrelloCard struct {
  Id string `json:id`
  Name string `json:name`
  Url string `json:url`
}

type TrelloCardsList struct {
  Id string `json:id`
  Name string `json:name`
  Cards []TrelloCard `json:cards`
}

func getTrelloCards(url string) *TrelloCardsList {
  res, err := http.Get(url)
  if err != nil {
    panic(err.Error())
  }
  body, err := ioutil.ReadAll(res.Body)
  if err != nil {
    panic(err.Error())
  }
  s := new(TrelloCardsList)
  err = json.Unmarshal(body, &s)
  return s
}

func main() {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("BOT_TOKEN"))
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

    trelloCardsList := getTrelloCards(cardsJsonUrl)
    buttonsRow := tgbotapi.NewInlineKeyboardRow(
      tgbotapi.NewInlineKeyboardButtonData("All right!", "/vote"),
    )
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("%v", trelloCardsList))
    msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttonsRow)
    msg.ReplyToMessageID = update.Message.MessageID

		bot.Send(msg)
	}
}

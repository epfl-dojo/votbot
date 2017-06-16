# VotBot
VotBot is a Telegram bot to organize votes. It is special in two points:
  1. It take a Trello Card List json url in parameter ;
  1. It's stateless: all data are stored in the chat itself.


## Run the bot
  * Use `make` to build votbot executable
  * Export your Telegram Bot Token before running the executable:  
    `export BOT_TOKEN=123456789:THISISATELEGRAMBOTTOKEN_@botfather; ./votbot`


## Usage
  * Talk to the bot ;
  * Create a new vote:  
    `/newvote https://raw.githubusercontent.com/epfl-dojo/votbot/master/minimal.json`
  * Do your vote ;
  * Close the vote:
    1. Reply the following to the vote message
    1. `/close`


## Minimal JSON
If you do not want to use a trello cards lists json url, you can use any json
file with, at least, these attributes:

```
{
    "name": "Name of the vote",
    "cards": [
        {
            "name": "First vote option"
        },
        {
            "name": "Second vote option"
        },
        {
            "name": "Third vote option"
        }
    ]
}
```

An example stands here: https://raw.githubusercontent.com/epfl-dojo/votbot/master/minimal.json


## Screenshot
![Screenshot](./screenshot.png)


## Operation Chart
![Operation Chart](./operation_votbot.png)

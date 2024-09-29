module CODStatusBot

go 1.23.1

require (
	github.com/bwmarrin/discordgo v0.28.1
	github.com/didip/tollbooth v4.0.2+incompatible
	github.com/gorilla/mux v1.8.1
	github.com/joho/godotenv v1.5.1
	github.com/sirupsen/logrus v1.9.3
	gorm.io/driver/mysql v1.5.7
	gorm.io/gorm v1.25.12
	CODStatusBot/models
	CODStatusBot/logger
	CODStatusBot/database
	CODStatusBot/utils

)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/go-sql-driver/mysql v1.8.1 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	golang.org/x/crypto v0.27.0 // indirect
	golang.org/x/sys v0.25.0 // indirect
	golang.org/x/text v0.18.0 // indirect
	golang.org/x/time v0.6.0 // indirect
)

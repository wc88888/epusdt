package telegram

import (
	"testing"

	"github.com/assimon/luuu/config"
)

func TestBotStartSkipsWhenTelegramConfigMissing(t *testing.T) {
	originalBot := bots
	originalToken := config.TgBotToken
	originalManage := config.TgManage

	t.Cleanup(func() {
		bots = originalBot
		config.TgBotToken = originalToken
		config.TgManage = originalManage
	})

	bots = nil
	config.TgBotToken = ""
	config.TgManage = 0

	BotStart()

	if bots != nil {
		t.Fatal("expected bot to remain nil when telegram config is missing")
	}
}

func TestSendToBotNoopWhenBotNotInitialized(t *testing.T) {
	originalBot := bots
	originalToken := config.TgBotToken
	originalManage := config.TgManage

	t.Cleanup(func() {
		bots = originalBot
		config.TgBotToken = originalToken
		config.TgManage = originalManage
	})

	bots = nil
	config.TgBotToken = "fake-token"
	config.TgManage = 123456

	SendToBot("hello")
}

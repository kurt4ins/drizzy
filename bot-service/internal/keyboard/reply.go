package keyboard

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

const (
	BtnBrowse  = "Смотреть анкеты ✨"
	BtnProfile = "Мой профиль 👤"
	BtnMatches = "Мэтчи 💛"
	BtnSkip    = "Пропустить"

	BtnPrefMale   = "Парней"
	BtnPrefFemale = "Девушек"
	BtnPrefAny    = "Всех"

	BtnGenderMale   = "Парень"
	BtnGenderFemale = "Девушка"

	BtnLike       = "❤️ Лайк"
	BtnBrowseSkip = "👎 Пропустить"

	BtnEditProfileFull = "Изменить профиль"
	BtnBack            = "Назад"
)

func MainMenu() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(BtnBrowse)),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(BtnProfile),
			tgbotapi.NewKeyboardButton(BtnMatches),
		),
	)
}

func SkipOnly() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(BtnSkip)),
	)
}

func PrefGender() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(BtnPrefMale),
			tgbotapi.NewKeyboardButton(BtnPrefFemale),
			tgbotapi.NewKeyboardButton(BtnPrefAny),
		),
	)
}

func GenderSelf() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(BtnGenderMale),
			tgbotapi.NewKeyboardButton(BtnGenderFemale),
		),
	)
}

// BrowseVote — только лайк, пропуск и выход в главное меню.
func BrowseVote() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(BtnLike),
			tgbotapi.NewKeyboardButton(BtnBrowseSkip),
		),
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(BtnBack)),
	)
}

// ProfileScreenMenu — просмотр профиля: перезаполнение анкеты или возврат в главное меню.
func ProfileScreenMenu() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(BtnEditProfileFull)),
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(BtnBack)),
	)
}

func Remove() tgbotapi.ReplyKeyboardRemove {
	return tgbotapi.NewRemoveKeyboard(true)
}

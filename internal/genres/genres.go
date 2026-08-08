// Package genres maps Flibusta / MyHomeLib genre codes to Russian titles.
package genres

import "strings"

// Name returns a human-readable genre title. Unknown codes are returned as-is.
func Name(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	if n, ok := names[code]; ok {
		return n
	}
	if n, ok := names[strings.ToLower(code)]; ok {
		return n
	}
	return code
}

// Known reports whether code has a Russian title in the built-in map.
func Known(code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	if _, ok := names[code]; ok {
		return true
	}
	_, ok := names[strings.ToLower(code)]
	return ok
}

var names = map[string]string{
	"sf":                    "Фантастика",
	"sf_history":            "Альтернативная история",
	"sf_action":             "Боевая фантастика",
	"sf_epic":               "Эпическая фантастика",
	"sf_heroic":             "Героическая фантастика",
	"sf_detective":          "Детективная фантастика",
	"sf_cyberpunk":          "Киберпанк",
	"sf_space":              "Космическая фантастика",
	"sf_social":             "Социальная фантастика",
	"sf_horror":             "Ужасы",
	"sf_humor":              "Юмористическая фантастика",
	"sf_fantasy":            "Фэнтези",
	"sf_fantasy_city":       "Городское фэнтези",
	"sf_fantasy_epic":       "Эпическое фэнтези",
	"sf_fantasy_dark":       "Тёмное фэнтези",
	"sf_mystic":             "Мистика",
	"sf_postapocalyptic":    "Постапокалипсис",
	"sf_litrpg":             "ЛитRPG",
	"sf_etc":                "Фантастика: прочее",
	"detective":             "Детектив",
	"detective_classic":     "Классический детектив",
	"detective_police":      "Полицейский детектив",
	"detective_action":      "Боевик",
	"detective_irony":       "Иронический детектив",
	"detective_history":     "Исторический детектив",
	"detective_espionage":   "Шпионский детектив",
	"detective_crime":       "Криминальный детектив",
	"detective_political":   "Политический детектив",
	"detective_maniac":      "Маньяки",
	"detective_hard":        "Крутой детектив",
	"thriller":              "Триллер",
	"prose":                 "Проза",
	"prose_classic":         "Классическая проза",
	"prose_history":         "Историческая проза",
	"prose_contemporary":    "Современная проза",
	"prose_counter":         "Контркультура",
	"prose_rus_classic":     "Русская классика",
	"prose_su_classic":      "Советская классика",
	"prose_military":        "О войне",
	"prose_game":            "Игры, спорт",
	"love":                  "Любовный роман",
	"love_contemporary":     "Современные любовные романы",
	"love_history":          "Исторические любовные романы",
	"love_detective":        "Остросюжетные любовные романы",
	"love_short":            "Короткие любовные романы",
	"love_erotica":          "Эротика",
	"love_sf":               "Любовно-фантастические романы",
	"adv":                   "Приключения",
	"adv_western":           "Вестерн",
	"adv_history":           "Исторические приключения",
	"adv_indian":            "Приключения про индейцев",
	"adv_maritime":          "Морские приключения",
	"adv_geo":               "Путешествия и география",
	"adv_animal":            "Природа и животные",
	"adv_modern":            "Приключения: современность",
	"adv_story":             "Авантюрный роман",
	"children":              "Детское",
	"child_tale":            "Сказка",
	"child_verse":           "Детские стихи",
	"child_prose":           "Детская проза",
	"child_sf":              "Детская фантастика",
	"child_det":             "Детские остросюжетные",
	"child_adv":             "Детские приключения",
	"child_education":       "Учебная литература",
	"poetry":                "Поэзия",
	"dramaturgy":            "Драматургия",
	"antique":               "Старинное",
	"antique_ant":           "Античная литература",
	"antique_european":      "Европейская старинная литература",
	"antique_russian":       "Древнерусская литература",
	"antique_east":          "Древневосточная литература",
	"antique_myths":         "Мифы. Легенды. Эпос",
	"sci":                   "Наука, образование",
	"sci_history":           "История",
	"sci_psychology":        "Психология",
	"sci_culture":           "Культурология",
	"sci_religion":          "Религиоведение",
	"sci_philosophy":        "Философия",
	"sci_politics":          "Политика",
	"sci_business":          "Деловая литература",
	"sci_juris":             "Юриспруденция",
	"sci_linguistic":        "Языкознание",
	"sci_medicine":          "Медицина",
	"sci_phys":              "Физика",
	"sci_math":              "Математика",
	"sci_chem":              "Химия",
	"sci_biology":           "Биология",
	"sci_tech":              "Технические науки",
	"sci_textbook":          "Учебники",
	"comp":                  "Компьютеры",
	"comp_www":              "Интернет",
	"comp_programming":      "Программирование",
	"comp_hard":             "Железо",
	"comp_soft":             "Программы",
	"comp_db":               "Базы данных",
	"comp_osnet":            "ОС и сети",
	"ref":                   "Справочники",
	"ref_encyc":             "Энциклопедии",
	"ref_dict":              "Словари",
	"ref_ref":               "Справочники",
	"ref_guide":             "Руководства",
	"nonf":                  "Документальное",
	"nonf_biography":        "Биографии и мемуары",
	"nonf_publicism":        "Публицистика",
	"nonf_criticism":        "Критика",
	"design":                "Искусство и Дизайн",
	"humor":                 "Юмор",
	"humor_anecdote":        "Анекдоты",
	"humor_prose":           "Юмористическая проза",
	"humor_verse":           "Юмористические стихи",
	"religion":              "Религия",
	"religion_rel":          "Религия",
	"religion_esoterics":    "Эзотерика",
	"religion_self":         "Самосовершенствование",
	"home":                  "Дом и семья",
	"home_cooking":          "Кулинария",
	"home_pets":             "Домашние животные",
	"home_crafts":           "Хобби и ремёсла",
	"home_entertain":        "Развлечения",
	"home_health":           "Здоровье",
	"home_garden":           "Сад и огород",
	"home_diy":              "Сделай сам",
	"home_sport":            "Спорт",
	"home_sex":              "Эротика, секс",
	"home_family":           "Семейные отношения",
	"home_antique":          "Коллекционирование",
	"foreign":               "Зарубежная литература",
	"foreign_prose":         "Зарубежная проза",
	"foreign_sf":            "Зарубежная фантастика",
	"foreign_detective":     "Зарубежный детектив",
	"foreign_love":          "Зарубежный любовный роман",
	"foreign_adventure":     "Зарубежные приключения",
	"foreign_contemporary":  "Современная зарубежная проза",
	"foreign_children":      "Зарубежная детская",
	"russian":               "Русская литература",
	"russian_contemporary":  "Современная русская литература",
	"network_literature":    "Самиздат, сетевая литература",
	"other": "Прочее",
	"notes": "Партитуры",
}

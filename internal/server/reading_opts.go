package server



import (

	"net/http"

	"strconv"



	"reading-assistant/internal/session"

)



func readingOptionsFromForm(r *http.Request) session.ReadingOptions {

	if r.Form.Has("showEasier") || r.Form.Has("showChinese") {

		return session.ReadingOptions{

			ShowEasier:  formBool(r, "showEasier"),

			ShowChinese: formBool(r, "showChinese"),

		}

	}

	if t := r.FormValue("track"); t != "" {

		return session.OptionsFromTrack(normalizeTrack(t))

	}

	if m := r.FormValue("readingMode"); m != "" {

		return session.OptionsFromMode(m)

	}

	return session.ReadingOptions{ShowEasier: true}

}



func formBool(r *http.Request, key string) bool {

	v := r.FormValue(key)

	return v == "true" || v == "1" || v == "on"

}



func readingOptionsFromJSON(showEasier, showChinese *bool, readingMode string) session.ReadingOptions {

	if showEasier != nil || showChinese != nil {

		o := session.ReadingOptions{}

		if showEasier != nil {

			o.ShowEasier = *showEasier

		}

		if showChinese != nil {

			o.ShowChinese = *showChinese

		}

		if !o.ShowEasier && !o.ShowChinese {

			o.ShowEasier = true

		}

		return o

	}

	if readingMode != "" {

		return session.OptionsFromMode(readingMode)

	}

	return session.ReadingOptions{ShowEasier: true}

}



func readingOptionsFromQuery(mode, showEasier, showChinese string) session.ReadingOptions {

	if showEasier != "" || showChinese != "" {

		e, _ := strconv.ParseBool(showEasier)

		c, _ := strconv.ParseBool(showChinese)

		return session.ReadingOptions{ShowEasier: e, ShowChinese: c}

	}

	if t := mode; t == session.TrackEasier || t == session.TrackChinese || t == session.TrackOriginal {

		return session.OptionsFromTrack(t)

	}

	if mode != "" {

		return session.OptionsFromMode(mode)

	}

	return session.ReadingOptions{ShowEasier: true}

}



func normalizeTrack(t string) string {

	switch t {

	case session.TrackChinese, session.TrackOriginal, session.TrackEasier:

		return t

	default:

		return session.OptionsFromMode(t).NavTrack()

	}

}



func exportBookQuery(opts session.ReadingOptions, showEasier, showChinese, mode string) string {

	if showEasier != "" || showChinese != "" {

		return "?showEasier=" + strconv.FormatBool(opts.ShowEasier) + "&showChinese=" + strconv.FormatBool(opts.ShowChinese)

	}

	if mode == session.TrackEasier || mode == session.TrackChinese || mode == session.TrackOriginal {

		return "?track=" + mode

	}

	if mode != "" {

		return "?mode=" + mode

	}

	if !opts.ShowEasier || opts.ShowChinese {

		return "?showEasier=" + strconv.FormatBool(opts.ShowEasier) + "&showChinese=" + strconv.FormatBool(opts.ShowChinese)

	}

	return ""

}



func mergeReadingOptions(base session.ReadingOptions, track string) session.ReadingOptions {

	if track == "" {

		return base

	}

	t := session.OptionsFromTrack(normalizeTrack(track))

	if track == session.TrackChinese || track == session.TrackOriginal || track == session.TrackEasier {

		return t

	}

	return base

}



//go:build linux

package gui

/*
#cgo linux pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <stdlib.h>
#include <string.h>

static GtkWidget *tux_window = NULL;
static GPtrArray *tux_parents = NULL;
static GHashTable *tux_inputs = NULL;
static char *tux_action = NULL;
static char *tux_target = NULL;
static char *tux_result = NULL;

static void tux_free_result_state(void) {
	if (tux_action != NULL) {
		g_free(tux_action);
		tux_action = NULL;
	}
	if (tux_target != NULL) {
		g_free(tux_target);
		tux_target = NULL;
	}
	if (tux_result != NULL) {
		g_free(tux_result);
		tux_result = NULL;
	}
	if (tux_inputs != NULL) {
		g_hash_table_destroy(tux_inputs);
		tux_inputs = NULL;
	}
	if (tux_parents != NULL) {
		g_ptr_array_free(tux_parents, TRUE);
		tux_parents = NULL;
	}
}

static void tux_json_append_escaped(GString *builder, const char *value) {
	const unsigned char *s = (const unsigned char *)(value == NULL ? "" : value);
	for (; *s != '\0'; ++s) {
		switch (*s) {
		case '\\':
			g_string_append(builder, "\\\\");
			break;
		case '"':
			g_string_append(builder, "\\\"");
			break;
		case '\n':
			g_string_append(builder, "\\n");
			break;
		case '\r':
			g_string_append(builder, "\\r");
			break;
		case '\t':
			g_string_append(builder, "\\t");
			break;
		default:
			if (*s < 0x20) {
				g_string_append_printf(builder, "\\u%04x", *s);
			} else {
				g_string_append_c(builder, *s);
			}
		}
	}
}

static char *tux_collect_values_json(void) {
	GString *builder = g_string_new("{");
	GHashTableIter iter;
	gpointer key = NULL;
	gpointer value = NULL;
	gboolean first = TRUE;

	g_hash_table_iter_init(&iter, tux_inputs);
	while (g_hash_table_iter_next(&iter, &key, &value)) {
		const char *entry_id = (const char *)key;
		GtkEntry *entry = GTK_ENTRY(value);
		const char *text = gtk_entry_get_text(entry);

		if (!first) {
			g_string_append(builder, ",");
		}
		first = FALSE;
		g_string_append(builder, "\"");
		tux_json_append_escaped(builder, entry_id);
		g_string_append(builder, "\":\"");
		tux_json_append_escaped(builder, text);
		g_string_append(builder, "\"");
	}

	g_string_append(builder, "}");
	return g_string_free(builder, FALSE);
}

static void tux_finalize_result(const char *action, const char *target) {
	char *values = tux_collect_values_json();
	GString *builder = g_string_new("{\"action\":\"");
	tux_json_append_escaped(builder, action);
	g_string_append(builder, "\",\"target\":\"");
	tux_json_append_escaped(builder, target);
	g_string_append(builder, "\",\"values\":");
	g_string_append(builder, values);
	g_string_append(builder, "}");

	if (tux_result != NULL) {
		g_free(tux_result);
	}
	tux_result = g_string_free(builder, FALSE);
	g_free(values);
}

static GtkWidget *tux_parent_at(int handle) {
	return GTK_WIDGET(g_ptr_array_index(tux_parents, handle));
}

static void tux_apply_css(GtkWidget *widget, const char *name, const char *fg, const char *bg, int font_size) {
	if ((fg == NULL || *fg == '\0') && (bg == NULL || *bg == '\0') && font_size <= 0) {
		return;
	}

	gtk_widget_set_name(widget, name);

	GString *css = g_string_new("");
	g_string_append_printf(css, "#%s {", name);
	if (fg != NULL && *fg != '\0') {
		g_string_append_printf(css, "color:%s;", fg);
	}
	if (bg != NULL && *bg != '\0') {
		g_string_append_printf(css, "background:%s;", bg);
	}
	if (font_size > 0) {
		g_string_append_printf(css, "font-size:%dpx;", font_size);
	}
	g_string_append(css, "}");

	GtkCssProvider *provider = gtk_css_provider_new();
	gtk_css_provider_load_from_data(provider, css->str, -1, NULL);
	gtk_style_context_add_provider(
		gtk_widget_get_style_context(widget),
		GTK_STYLE_PROVIDER(provider),
		GTK_STYLE_PROVIDER_PRIORITY_USER
	);

	g_object_unref(provider);
	g_string_free(css, TRUE);
}

static void tux_on_window_destroy(GtkWidget *widget, gpointer data) {
	if (tux_action == NULL) {
		tux_action = g_strdup("close");
	}
	if (tux_target == NULL) {
		tux_target = g_strdup("");
	}
	tux_finalize_result(tux_action, tux_target);
	gtk_main_quit();
}

static void tux_on_button_clicked(GtkWidget *widget, gpointer data) {
	const char *target = (const char *)data;

	if (tux_action != NULL) {
		g_free(tux_action);
	}
	if (tux_target != NULL) {
		g_free(tux_target);
	}
	tux_action = g_strdup("click");
	tux_target = g_strdup(target == NULL ? "" : target);
	tux_finalize_result(tux_action, tux_target);
	gtk_main_quit();
}

static gboolean tux_on_button_right_click(GtkWidget *widget, GdkEventButton *event, gpointer data) {
	if (event == NULL || event->type != GDK_BUTTON_PRESS || event->button != 3) {
		return FALSE;
	}

	const char *target = (const char *)data;

	if (tux_action != NULL) {
		g_free(tux_action);
	}
	if (tux_target != NULL) {
		g_free(tux_target);
	}
	tux_action = g_strdup("rightclick");
	tux_target = g_strdup(target == NULL ? "" : target);
	tux_finalize_result(tux_action, tux_target);
	gtk_main_quit();
	return TRUE;
}

static int tux_gtk_init(void) {
	int argc = 0;
	char **argv = NULL;
	return gtk_init_check(&argc, &argv);
}

static void tux_init_window(const char *title, int x, int y, int width, int height, const char *background) {
	tux_free_result_state();

	tux_inputs = g_hash_table_new_full(g_str_hash, g_str_equal, g_free, NULL);
	tux_parents = g_ptr_array_new();

	tux_window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
	gtk_window_set_title(GTK_WINDOW(tux_window), title == NULL ? "" : title);
	gtk_window_set_default_size(GTK_WINDOW(tux_window), width, height);
	gtk_window_move(GTK_WINDOW(tux_window), x, y);
	g_signal_connect(tux_window, "destroy", G_CALLBACK(tux_on_window_destroy), NULL);

	GtkWidget *root = gtk_fixed_new();
	gtk_container_add(GTK_CONTAINER(tux_window), root);
	g_ptr_array_add(tux_parents, root);

	tux_apply_css(tux_window, "tux_window", "", background, 0);
}

static int tux_add_panel(int parent, const char *name, int x, int y, int width, int height, const char *background) {
	GtkWidget *panel = gtk_fixed_new();
	gtk_widget_set_size_request(panel, width, height);
	gtk_fixed_put(GTK_FIXED(tux_parent_at(parent)), panel, x, y);
	tux_apply_css(panel, name, "", background, 0);
	g_ptr_array_add(tux_parents, panel);
	return (int)tux_parents->len - 1;
}

static void tux_add_label(int parent, const char *name, const char *text, int x, int y, int width, int height, const char *fg, const char *bg, int font_size) {
	GtkWidget *label = gtk_label_new(text == NULL ? "" : text);
	gtk_label_set_xalign(GTK_LABEL(label), 0.0f);
	gtk_widget_set_size_request(label, width, height);
	gtk_fixed_put(GTK_FIXED(tux_parent_at(parent)), label, x, y);
	tux_apply_css(label, name, fg, bg, font_size);
}

static void tux_add_button(int parent, const char *name, const char *target, const char *text, int x, int y, int width, int height, const char *fg, const char *bg, int font_size) {
	GtkWidget *button = gtk_button_new_with_label(text == NULL ? "" : text);
	gtk_widget_set_size_request(button, width, height);
	gtk_fixed_put(GTK_FIXED(tux_parent_at(parent)), button, x, y);
	g_signal_connect(button, "clicked", G_CALLBACK(tux_on_button_clicked), g_strdup(target == NULL ? "" : target));
	gtk_widget_add_events(button, GDK_BUTTON_PRESS_MASK);
	g_signal_connect(button, "button-press-event", G_CALLBACK(tux_on_button_right_click), g_strdup(target == NULL ? "" : target));
	tux_apply_css(button, name, fg, bg, font_size);
}

static void tux_add_entry(int parent, const char *name, const char *entry_id, const char *text, const char *placeholder, int x, int y, int width, int height, const char *fg, const char *bg, int font_size) {
	GtkWidget *entry = gtk_entry_new();
	gtk_entry_set_text(GTK_ENTRY(entry), text == NULL ? "" : text);
	if (placeholder != NULL && *placeholder != '\0') {
		gtk_entry_set_placeholder_text(GTK_ENTRY(entry), placeholder);
	}
	gtk_widget_set_size_request(entry, width, height);
	gtk_fixed_put(GTK_FIXED(tux_parent_at(parent)), entry, x, y);
	g_hash_table_insert(tux_inputs, g_strdup(entry_id == NULL ? "" : entry_id), entry);
	tux_apply_css(entry, name, fg, bg, font_size);
}

static char *tux_run_window(void) {
	gtk_widget_show_all(tux_window);
	gtk_main();

	if (tux_result == NULL) {
		tux_finalize_result("close", "");
	}

	char *out = g_strdup(tux_result);
	if (GTK_IS_WIDGET(tux_window)) {
		gtk_widget_destroy(tux_window);
	}
	tux_window = NULL;
	tux_free_result_state();
	return out;
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

func showNativeDesktop(w *Window) (*Result, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if C.tux_gtk_init() == 0 {
		return nil, fmt.Errorf("GTK3 is not available")
	}

	title := cString(w.title)
	background := cString(w.background)
	defer freeCString(title)
	defer freeCString(background)

	C.tux_init_window(title, C.int(w.x), C.int(w.y), C.int(w.width), C.int(w.height), background)
	if err := renderLinuxWidgets(0, w.children); err != nil {
		return nil, err
	}

	raw := C.tux_run_window()
	defer C.free(unsafe.Pointer(raw))
	return decodeResult([]byte(C.GoString(raw)))
}

func renderLinuxWidgets(parent int, widgets []Widget) error {
	for i, widget := range widgets {
		base := widget.base()
		id := base.id
		if id == "" {
			id = fmt.Sprintf("%s_%d", widget.kind(), i)
		}
		name := fmt.Sprintf("tux_%s_%d", widget.kind(), i)

		switch value := widget.(type) {
		case *Label:
			if err := linuxAddLabel(parent, name, value.text, base); err != nil {
				return err
			}
		case *Button:
			if err := linuxAddButton(parent, name, id, value.text, base); err != nil {
				return err
			}
		case *TextInput:
			if err := linuxAddEntry(parent, name, id, value.text, value.placeholder, base); err != nil {
				return err
			}
		case *Panel:
			handle, err := linuxAddPanel(parent, name, base)
			if err != nil {
				return err
			}
			if err := renderLinuxWidgets(handle, value.children); err != nil {
				return err
			}
		}
	}
	return nil
}

func linuxAddPanel(parent int, name string, base *baseWidget) (int, error) {
	cName := cString(name)
	cBackground := cString(base.background)
	defer freeCString(cName)
	defer freeCString(cBackground)

	handle := C.tux_add_panel(
		C.int(parent),
		cName,
		C.int(base.x),
		C.int(base.y),
		C.int(base.width),
		C.int(base.height),
		cBackground,
	)
	return int(handle), nil
}

func linuxAddLabel(parent int, name string, text string, base *baseWidget) error {
	cName := cString(name)
	cText := cString(text)
	cForeground := cString(base.foreground)
	cBackground := cString(base.background)
	defer freeCString(cName)
	defer freeCString(cText)
	defer freeCString(cForeground)
	defer freeCString(cBackground)

	C.tux_add_label(
		C.int(parent),
		cName,
		cText,
		C.int(base.x),
		C.int(base.y),
		C.int(base.width),
		C.int(base.height),
		cForeground,
		cBackground,
		C.int(base.fontSize),
	)
	return nil
}

func linuxAddButton(parent int, name string, id string, text string, base *baseWidget) error {
	cName := cString(name)
	cID := cString(id)
	cText := cString(text)
	cForeground := cString(base.foreground)
	cBackground := cString(base.background)
	defer freeCString(cName)
	defer freeCString(cID)
	defer freeCString(cText)
	defer freeCString(cForeground)
	defer freeCString(cBackground)

	C.tux_add_button(
		C.int(parent),
		cName,
		cID,
		cText,
		C.int(base.x),
		C.int(base.y),
		C.int(base.width),
		C.int(base.height),
		cForeground,
		cBackground,
		C.int(base.fontSize),
	)
	return nil
}

func linuxAddEntry(parent int, name string, id string, text string, placeholder string, base *baseWidget) error {
	cName := cString(name)
	cID := cString(id)
	cText := cString(text)
	cPlaceholder := cString(placeholder)
	cForeground := cString(base.foreground)
	cBackground := cString(base.background)
	defer freeCString(cName)
	defer freeCString(cID)
	defer freeCString(cText)
	defer freeCString(cPlaceholder)
	defer freeCString(cForeground)
	defer freeCString(cBackground)

	C.tux_add_entry(
		C.int(parent),
		cName,
		cID,
		cText,
		cPlaceholder,
		C.int(base.x),
		C.int(base.y),
		C.int(base.width),
		C.int(base.height),
		cForeground,
		cBackground,
		C.int(base.fontSize),
	)
	return nil
}

func cString(value string) *C.char {
	return C.CString(value)
}

func freeCString(value *C.char) {
	C.free(unsafe.Pointer(value))
}

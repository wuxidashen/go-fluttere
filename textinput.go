package flutter

import (
	"encoding/json"
	"fmt"

	"github.com/go-flutter-desktop/go-flutter/plugin"
	"github.com/go-gl/glfw/v3.2/glfw"
	"github.com/pkg/errors"
)

const textinputChannelName = "flutter/textinput"

// textinputPlugin implements flutter.Plugin and handles method calls to the
// flutter/textinput channel.
type textinputPlugin struct {
	messenger plugin.BinaryMessenger
	window    *glfw.Window
	channel   *plugin.MethodChannel

	keyboardLayout KeyboardShortcuts

	modifierKey           glfw.ModifierKey
	wordTravellerKey      glfw.ModifierKey
	wordTravellerKeyShift glfw.ModifierKey

	clientID        float64
	clientConf      argSetClientConf
	word            []rune
	selectionBase   int
	selectionExtent int
}

// keyboardShortcutsGLFW handle glfw.ModifierKey from glfwKeyCallback.
type keyboardShortcutsGLFW struct {
	mod glfw.ModifierKey
}

// all hardcoded because theres not pluggable renderer system.
var defaultTextinputPlugin = &textinputPlugin{}

var _ Plugin = &textinputPlugin{}     // compile-time type check
var _ PluginGLFW = &textinputPlugin{} // compile-time type check

func (p *textinputPlugin) InitPlugin(messenger plugin.BinaryMessenger) error {
	p.messenger = messenger

	return nil
}

func (p *textinputPlugin) InitPluginGLFW(window *glfw.Window) error {
	p.window = window
	p.channel = plugin.NewMethodChannel(p.messenger, textinputChannelName, plugin.JSONMethodCodec{})
	p.channel.HandleFuncSync("TextInput.setClient", p.handleSetClient)
	p.channel.HandleFuncSync("TextInput.clearClient", p.handleClearClient)
	p.channel.HandleFuncSync("TextInput.setEditingState", p.handleSetEditingState)

	return nil
}

func (p *textinputPlugin) handleSetClient(arguments interface{}) (reply interface{}, err error) {
	args := []json.RawMessage{}
	err = json.Unmarshal(arguments.(json.RawMessage), &args)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode json arguments for handleSetClient")
	}

	err = json.Unmarshal(args[0], &p.clientID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode clientID for handleSetClient")
	}

	err = json.Unmarshal(args[1], &p.clientConf)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode clientConf for handleSetClient")
	}

	return nil, nil
}

func (p *textinputPlugin) handleClearClient(arguments interface{}) (reply interface{}, err error) {
	p.clientID = 0
	return nil, nil
}

func (p *textinputPlugin) handleSetEditingState(arguments interface{}) (reply interface{}, err error) {
	if p.clientID == 0 {
		return nil, errors.New("cannot set editing state when no client is selected")
	}

	editingState := argsEditingState{}
	err = json.Unmarshal(arguments.(json.RawMessage), &editingState)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode json arguments for handleSetEditingState")
	}

	p.word = []rune(editingState.Text)
	p.selectionBase = editingState.SelectionBase
	p.selectionExtent = editingState.SelectionExtent
	return nil, nil
}

func (p *textinputPlugin) glfwCharCallback(w *glfw.Window, char rune) {
	if p.clientID == 0 {
		return
	}
	p.addChar([]rune{char})
}

func (p *textinputPlugin) glfwKeyCallback(window *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {

	keyboardShortcutBind := keyboardShortcutsGLFW{mod: mods}
	if key == glfw.KeyEscape && action == glfw.Press {
		_, err := defaultNavigationPlugin.channel.InvokeMethod("popRoute", nil)
		if err != nil {
			fmt.Printf("go-flutter: failed to pop route after escape key press: %v\n", err)
		}
		return
	}

	if action == glfw.Repeat || action == glfw.Press {
		if p.clientID == 0 {
			return
		}

		switch key {
		case glfw.KeyEnter:
			if keyboardShortcutBind.isModifier() {
				// Indicates that they are done typing in the TextInput
				p.performAction("TextInputAction.done")
				return
			} else if p.clientConf.InputType.Name == "TextInputType.multiline" {
				p.addChar([]rune{'\n'})
			}
			p.performTextInputAction()

		case glfw.KeyHome:
			p.MoveCursorHome(keyboardShortcutBind)

		case glfw.KeyEnd:
			p.MoveCursorEnd(keyboardShortcutBind)

		case glfw.KeyLeft:
			p.MoveCursorLeft(keyboardShortcutBind)

		case glfw.KeyRight:
			p.MoveCursorRight(keyboardShortcutBind)

		case glfw.KeyDelete:
			p.Delete(keyboardShortcutBind)

		case glfw.KeyBackspace:
			p.Backspace(keyboardShortcutBind)

		case p.keyboardLayout.SelectAll:
			if keyboardShortcutBind.isModifier() {
				p.selectAll()
			}

		case p.keyboardLayout.Copy:
			if keyboardShortcutBind.isModifier() && p.isSelected() {
				_, _, selectedContent := p.getSelectedText()
				window.SetClipboardString(selectedContent)
			}

		case p.keyboardLayout.Cut:
			if keyboardShortcutBind.isModifier() && p.isSelected() {
				_, _, selectedContent := p.getSelectedText()
				window.SetClipboardString(selectedContent)
				p.removeSelectedText()
			}

		case p.keyboardLayout.Paste:
			if keyboardShortcutBind.isModifier() {
				var clpString, err = window.GetClipboardString()
				if err != nil {
					fmt.Printf("go-flutter: unable to get the clipboard content: %v\n", err)
					return
				}
				p.addChar([]rune(clpString))
			}
		}
		p.updateEditingState()
	}
}

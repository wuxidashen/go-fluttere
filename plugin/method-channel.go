package plugin

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"
)

// MethodHandler defines the interface for a method handler.
type MethodHandler interface {
	// HandleMethod is called whenever an incoming
	HandleMethod(arguments interface{}) (reply interface{}, err error)
}

// The MethodHandlerFunc type is an adapter to allow the use of
// ordinary functions as method handlers. If f is a function
// with the appropriate signature, MethodHandlerFunc(f) is a
// MethodHandler that calls f.
type MethodHandlerFunc func(arguments interface{}) (reply interface{}, err error)

// HandleMethod calls f(arguments).
func (f MethodHandlerFunc) HandleMethod(arguments interface{}) (reply interface{}, err error) {
	return f(arguments)
}

// MethodChannel provides way for flutter applications and hosts to communicate.
// It must be used with a codec, for example the StandardMethodCodec. For more
// information please read
// https://flutter.dev/docs/development/platform-integration/platform-channels
type MethodChannel struct {
	messenger   BinaryMessenger
	channelName string
	methodCodec MethodCodec

	methods     map[string]MethodHandler
	methodsLock sync.RWMutex
}

// NewMethodChannel creates a new method channel
func NewMethodChannel(messenger BinaryMessenger, channelName string, methodCodec MethodCodec) (channel *MethodChannel) {
	mc := &MethodChannel{
		messenger:   messenger,
		channelName: channelName,
		methodCodec: methodCodec,

		methods: make(map[string]MethodHandler),
	}
	messenger.SetChannelHandler(channelName, mc.handleChannel)
	return mc
}

// InvokeMethod sends a methodcall to the binary messenger and waits for a
// result.
func (m *MethodChannel) InvokeMethod(name string, arguments interface{}) (result interface{}, err error) {
	encodedMessage, err := m.methodCodec.EncodeMethodCall(MethodCall{
		Method:    name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode methodcall")
	}
	encodedReply, err := m.messenger.Send(m.channelName, encodedMessage)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send methodcall")
	}
	result, err = m.methodCodec.DecodeEnvelope(encodedReply)
	if flutterError, ok := err.(*FlutterError); ok {
		return nil, flutterError
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Handle registers a message handler on this channel for receiving messages
// sent from the Flutter application.
//
// Consecutive calls override any existing handler registration for (the name
// of) this channel. When given nil as handler, the previously registered
// handler for a method is unregistrered.
//
// When no handler is registered for a method, it will be handled silently by
// sending a nil reply (null on the dart side).
func (m *MethodChannel) Handle(methodName string, handler MethodHandler) {
	m.methodsLock.Lock()
	if handler == nil {
		delete(m.methods, methodName)
	} else {
		m.methods[methodName] = handler
	}
	m.methodsLock.Unlock()
}

// HandleFunc is a shorthand for m.Handle(MethodHandlerFunc(f))
func (m *MethodChannel) HandleFunc(methodName string, f func(arguments interface{}) (reply interface{}, err error)) {
	if f == nil {
		m.Handle(methodName, nil)
		return
	}

	m.Handle(methodName, MethodHandlerFunc(f))
}

// handleChannel decodes incoming binary message to a method call, calls the
// handler, and encodes the outgoing reply.
func (m *MethodChannel) handleChannel(binaryMessage []byte) (binaryReply []byte, err error) {
	methodCall, err := m.methodCodec.DecodeMethodCall(binaryMessage)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode incomming message")
	}
	m.methodsLock.RLock()
	methodHandler := m.methods[methodCall.Method]
	m.methodsLock.RUnlock()
	if methodHandler == nil {
		fmt.Printf("go-flutter: no method handler registered for method '%s' on channel '%s'\n", methodCall.Method, m.channelName)
		// return nil as reply, which may be sent back to the dart side
		return nil, nil
	}

	defer func() {
		p := recover()
		if p != nil {
			fmt.Printf("go-flutter: recovered from panic while handling call for method '%s' on channel '%s': %v", methodCall.Method, m.channelName, p)
		}
	}()
	reply, err := methodHandler.HandleMethod(methodCall.Arguments)
	if err != nil {
		fmt.Printf("go-flutter: handler for method '%s' on channel '%s' returned an error: %v\n", methodCall.Method, m.channelName, err)
		binaryReply, err = m.methodCodec.EncodeErrorEnvelope("error", err.Error(), nil)
		if err != nil {
			return nil, errors.Wrap(err, "failed to encode error envelope")
		}
		return binaryReply, nil
	}

	binaryReply, err = m.methodCodec.EncodeSuccessEnvelope(reply)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode success envelope")
	}

	return binaryReply, nil
}

var _ ChannelHandlerFunc = (*MethodChannel)(nil).handleChannel // compile-time type check

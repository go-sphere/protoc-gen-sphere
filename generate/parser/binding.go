package parser

import (
	bindingpb "github.com/go-sphere/binding/sphere/binding"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
)

func checkBindingLocation(message *protogen.Message, field *protogen.Field, location bindingpb.BindingLocation) bool {
	if proto.HasExtension(field.Desc.Options(), bindingpb.E_Location) {
		bindingLocation := proto.GetExtension(field.Desc.Options(), bindingpb.E_Location).(bindingpb.BindingLocation)
		return bindingLocation == location
	}
	if proto.HasExtension(message.Desc.Options(), bindingpb.E_DefaultLocation) {
		defaultBindingLocation := proto.GetExtension(message.Desc.Options(), bindingpb.E_DefaultLocation).(bindingpb.BindingLocation)
		return defaultBindingLocation == location
	}
	return false
}

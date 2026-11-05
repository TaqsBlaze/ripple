import QtQuick
import QtQuick.Controls

Button {
    id: control
    property bool selected: false
    property bool compact: false
    ToolTip.visible: compact && hovered
    ToolTip.text: text
    property string symbol: "○"
    implicitHeight: 48
    leftPadding: compact ? 12 : 16
    rightPadding: compact ? 12 : 16
    font.pixelSize: 14
    font.weight: selected ? Font.DemiBold : Font.Normal

    contentItem: Row {
        spacing: 12
        Text { text: control.symbol; color: control.selected ? "#ffffff" : "#8d8d8d"; font.pixelSize: 20; width: 24; horizontalAlignment: Text.AlignHCenter }
        Text { visible: !control.compact; text: control.text; color: control.selected ? "#ffffff" : "#a3a3a3"; font: control.font; anchors.verticalCenter: parent.verticalCenter }
    }
    background: Rectangle {
        radius: 12
        color: control.selected ? "#242424" : control.hovered ? "#1b1b1b" : "transparent"
        border.color: control.selected ? "#333333" : "transparent"
    }
}

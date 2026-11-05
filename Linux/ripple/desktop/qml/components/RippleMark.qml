import QtQuick

Item {
    implicitWidth: 42
    implicitHeight: 42
    Image {
        anchors.fill: parent
        source: "qrc:/assets/icon.png"
        fillMode: Image.PreserveAspectFit
        smooth: true
        mipmap: true
    }
}

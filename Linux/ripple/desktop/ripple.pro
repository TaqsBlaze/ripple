QT += quick quickcontrols2 network
CONFIG += c++17

TARGET = ripple-desktop
TEMPLATE = app

SOURCES += \
    src/main.cpp \
    src/ipcclient.cpp

HEADERS += src/ipcclient.h

RESOURCES += qml.qrc

QMAKE_CXXFLAGS += -Wall -Wextra -Wpedantic

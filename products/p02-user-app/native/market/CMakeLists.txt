cmake_minimum_required(VERSION 3.22.1)
project(appmodules LANGUAGES C CXX)
include(${REACT_ANDROID_DIR}/cmake-utils/ReactNative-application.cmake)

add_library(nu54_market_lab SHARED wallet.c crypto.c jni.c)
set_target_properties(nu54_market_lab PROPERTIES C_STANDARD 11 C_STANDARD_REQUIRED YES)
target_compile_options(nu54_market_lab PRIVATE -fvisibility=hidden)
target_link_options(nu54_market_lab PRIVATE -Wl,-z,max-page-size=16384)

# P01 payment signer firmware (DF-20260925-02, [D05][N25]).
cmake_minimum_required(VERSION 3.20.0)

# Board package vendored under ../boards (see boards/nucode/nu54v_dk/VENDORED.md).
list(APPEND BOARD_ROOT ${CMAKE_CURRENT_LIST_DIR}/..)

find_package(Zephyr REQUIRED HINTS $ENV{ZEPHYR_BASE})
project(nu54_signer)

# Generated protocol constants shared with the kiosk, operator tool and contracts.
target_include_directories(app PRIVATE ${CMAKE_CURRENT_LIST_DIR}/../../../packages/protocol/c/include)

# Board-independent core, also built and tested on the host (see ../test).
target_include_directories(app PRIVATE ${CMAKE_CURRENT_LIST_DIR}/../core/include)

target_sources(app PRIVATE
  src/main.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_frame.c
)

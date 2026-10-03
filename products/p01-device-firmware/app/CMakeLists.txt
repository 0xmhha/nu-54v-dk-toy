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
  src/board_io.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_frame.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_button.c
  src/device_key.c
  src/device_setup.c
  src/pay_link.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_keccak.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_eip712.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_sig.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_cbor.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_session.c
  ${CMAKE_CURRENT_LIST_DIR}/../third_party/keccak/Keccak-readable-and-compact.c
)

# libsecp256k1 (recovery for operator and merchant signatures, low-s and v): same settings as
# the host tests in ../test/CMakeLists.txt.
set(SECP ${CMAKE_CURRENT_LIST_DIR}/../third_party/secp256k1)
zephyr_library_named(nu54_secp256k1)
zephyr_library_sources(${SECP}/src/secp256k1.c ${SECP}/src/precomputed_ecmult.c ${SECP}/src/precomputed_ecmult_gen.c)
zephyr_library_include_directories(${SECP}/src ${SECP}/include)
zephyr_library_compile_definitions(ENABLE_MODULE_RECOVERY=1 ECMULT_WINDOW_SIZE=4)
# Upstream builds the precomputed tables with unused-function warnings off as well.
zephyr_library_compile_options(-Wno-unused-function)
target_include_directories(app PRIVATE ${SECP}/include)
target_link_libraries(app PRIVATE nu54_secp256k1)

# Development fixed setup (week 7): operator, settlement contract and chain id from the
# deployment record, generated at build time and never typed by hand.
set(DEPLOYMENT ${CMAKE_CURRENT_LIST_DIR}/../../p06-stablenet-contracts/deployments/8283.json)
set(DEV_SETUP_H ${CMAKE_CURRENT_BINARY_DIR}/dev_setup/dev_setup.h)
add_custom_command(OUTPUT ${DEV_SETUP_H}
  COMMAND ${PYTHON_EXECUTABLE} ${CMAKE_CURRENT_LIST_DIR}/gen_dev_setup.py ${DEPLOYMENT} ${DEV_SETUP_H}
  DEPENDS ${CMAKE_CURRENT_LIST_DIR}/gen_dev_setup.py ${DEPLOYMENT})
add_custom_target(dev_setup DEPENDS ${DEV_SETUP_H})
add_dependencies(app dev_setup)
target_include_directories(app PRIVATE ${CMAKE_CURRENT_BINARY_DIR}/dev_setup)

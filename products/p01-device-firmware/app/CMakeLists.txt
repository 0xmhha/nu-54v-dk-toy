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
if(CONFIG_NU54_COUNTED_UPDATE)
  target_sources(app PRIVATE src/device_update.c)
  target_include_directories(app PRIVATE "${ZEPHYR_MCUBOOT_MODULE_DIR}/boot/bootutil/include"
                                           "${ZEPHYR_MCUBOOT_MODULE_DIR}/boot/zephyr/include")
endif()
target_sources_ifdef(CONFIG_NU54_HD_RECOVERY app PRIVATE src/device_hd.c src/device_evm.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_market_lab.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_accessory.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_hd.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_hd_slot.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_recovery.c)
if(CONFIG_NU54_WALLET_CHAINS)
  set(NU54_TF_PSA_DIR "${ZEPHYR_OBERON_PSA_CRYPTO_MODULE_DIR}/../tf-psa-crypto")
  set_source_files_properties(src/device_ripemd160.c PROPERTIES INCLUDE_DIRECTORIES
    "${NU54_TF_PSA_DIR}/core;${NU54_TF_PSA_DIR}/drivers/builtin/include;${NU54_TF_PSA_DIR}/drivers/builtin/src")
  target_sources(app PRIVATE src/device_ripemd160.c src/device_chains.c
    ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_btc.c
    ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_btc48.c
    ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_solana.c
    ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_xrpl.c)
endif()
target_sources_ifdef(CONFIG_NU54_SPENDING_PROFILE app PRIVATE src/device_order.c src/device_spending.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_order.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_spending.c)
target_sources_ifdef(CONFIG_NU54_PILOT_TLS app PRIVATE src/device_tls_v2.c src/device_pilot.c
  src/device_install.c src/device_recovery_pin.c ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_install.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_running_firmware.c
  src/device_firmware_power.c ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_firmware_power.c)
if(DEFINED NU54_PILOT_PROVISION OR DEFINED ENV{NU54_PILOT_PROVISION})
  message(FATAL_ERROR "Compiled-in pilot credentials have been replaced by signed runtime enrollment")
endif()

if(DEFINED ENV{NU54_OFFLINE_RECOVERY_PROVISION})
  message(FATAL_ERROR "Use an authority-signed runtime recovery trust grant")
endif()

target_sources(app PRIVATE
  src/main.c
  src/board_io.c
  # Compile the portable v2 SDK for this target without activating its TLS/BLE backend.
  ${CMAKE_CURRENT_LIST_DIR}/../../../packages/protocol/c/src/nu54_quote_v2.c
  ${CMAKE_CURRENT_LIST_DIR}/../../../packages/protocol/c/src/nu54_transport_v2.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_frame.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_button.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_pin_entry.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_pin_attempt.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_local_approval.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_pilot.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_evm_tx.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_siwe.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_owner.c
  src/device_key.c
  src/device_setup.c
  src/device_vault.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_vault.c
  src/pay_link.c
  src/ble_links.c
  src/status_led.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_keccak.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_eip712.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_sig.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_secure.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_cbor.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_session.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_session_util.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_session_channel.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_session_setup.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_session_pay.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_session_limit.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_rental_journal.c
  # The session core checks rental credentials in every image; only the store is optional.
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_rental_credential.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_links.c
  ${CMAKE_CURRENT_LIST_DIR}/../third_party/keccak/Keccak-readable-and-compact.c
)

# libsecp256k1 (recovery for operator and merchant signatures, low-s and v): same settings as
# the host tests in ../test/CMakeLists.txt.
set(SECP ${CMAKE_CURRENT_LIST_DIR}/../third_party/secp256k1)
zephyr_library_named(nu54_secp256k1)
zephyr_library_sources(${SECP}/src/secp256k1.c ${SECP}/src/precomputed_ecmult.c ${SECP}/src/precomputed_ecmult_gen.c)
zephyr_library_include_directories(${SECP}/src ${SECP}/include)
zephyr_library_compile_definitions(ENABLE_MODULE_RECOVERY=1 ENABLE_MODULE_ECDH=1 ECMULT_WINDOW_SIZE=4)
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

# The experimental rental adapter refuses pending cleanup unless separately qualified.
# It provides no key retirement or completion receipt.
target_sources_ifdef(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS app PRIVATE
  src/device_rental_journal_ps.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_rental_retirement.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_rental_freeze.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_rental_registration.c
)

target_sources_ifdef(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA app PRIVATE
  src/device_owner_limits_ps.c src/device_owner_finality_ps.c ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_owner_finality_enroll.c ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_owner_finality.c ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_owner_limits.c
  ${CMAKE_CURRENT_LIST_DIR}/../core/src/nu54_owner_limit_change.c)

if(CONFIG_NU54_EXPERIMENTAL_W27_QUALIFICATION)
  set(W27Q_SOURCE "${CMAKE_CURRENT_LIST_DIR}/experimental/w27_qualification")
  set(W27Q_GENERATED "${CMAKE_CURRENT_BINARY_DIR}/w27_qualification")
  get_target_property(TFM_BINARY_DIR tfm TFM_BINARY_DIR)
  configure_file("${W27Q_SOURCE}/manifest_list.yaml.in" "${W27Q_GENERATED}/manifest_list.yaml")
  set(W27Q_TAG_INPUTS "${ZEPHYR_BINARY_DIR}/.config" "${ZEPHYR_BINARY_DIR}/zephyr.dts"
    "${W27Q_SOURCE}/partition.yaml" "${W27Q_SOURCE}/CMakeLists.txt" "${W27Q_SOURCE}/w27_qualification.h"
    "${W27Q_SOURCE}/w27_qualification_scope.h" "${W27Q_SOURCE}/secure.c" "${W27Q_SOURCE}/nonsecure.c" "${ZEPHYR_NRF_MODULE_DIR}/VERSION")
  set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS ${W27Q_TAG_INPUTS})
  set(W27Q_TAG_MATERIAL "nu54-w27-nrf54l15-v2")
  foreach(input IN LISTS W27Q_TAG_INPUTS)
    file(SHA256 "${input}" digest)
    string(APPEND W27Q_TAG_MATERIAL "${digest}")
  endforeach()
  string(SHA256 W27Q_IMAGE "${W27Q_TAG_MATERIAL}")
  file(CONFIGURE OUTPUT "${W27Q_GENERATED}/w27_qualification_image.h"
    CONTENT "#define NU54_W27Q_IMAGE \"${W27Q_IMAGE}\"\n")
  set_property(TARGET zephyr_property_target APPEND PROPERTY TFM_CMAKE_OPTIONS
    "-DTFM_EXTRA_MANIFEST_LIST_FILES=${W27Q_GENERATED}/manifest_list.yaml"
    "-DTFM_EXTRA_PARTITION_PATHS=${W27Q_SOURCE}"
    "-DNU54_W27Q_CONFIG_DIR=${W27Q_GENERATED}")
  target_sources(app PRIVATE "${W27Q_SOURCE}/nonsecure.c")
  target_include_directories(app PRIVATE "${W27Q_GENERATED}" "${CMAKE_CURRENT_LIST_DIR}/src"
    "$<TARGET_PROPERTY:tfm,TFM_BINARY_DIR>/api_ns/interface/include")
endif()

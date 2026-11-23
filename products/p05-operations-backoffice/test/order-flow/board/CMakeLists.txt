# The spending wallet firmware compiled for the host (board_host.c). Built by scripts/test_order_flow.py.
cmake_minimum_required(VERSION 3.20)
project(nu54_order_flow_board C)

find_package(Python3 REQUIRED COMPONENTS Interpreter)
find_package(OpenSSL REQUIRED)
get_filename_component(REPO ${CMAKE_CURRENT_SOURCE_DIR}/../../../../.. ABSOLUTE)
set(P01 ${REPO}/products/p01-device-firmware)
set(PROTOCOL ${REPO}/packages/protocol/c)
set(SECP ${P01}/third_party/secp256k1)

# The message handling is taken out of the app sources, so this board runs the text the real one builds.
set(EXTRACTED ${CMAKE_CURRENT_BINARY_DIR}/board_pilot.h)
add_custom_command(OUTPUT ${EXTRACTED}
  COMMAND ${Python3_EXECUTABLE} -B ${P01}/test/extract_c.py ${EXTRACTED}
    ${P01}/app/src/device_install.c import_public verify device_install_verify_authority
    ${P01}/app/src/device_pilot.c RESET_NONCE_MS reset_display_input reset_install_input reset_link
      session_live owner_pin_live pilot_pin send_frame present load_refusal clear_legacy_bonds report_legacy_bonds
      reset_nonce_issue journal_new_session spending_request spending_erase_and_reboot spending_button manage load_receipt send_receipt
      reclaim_receipt receipt_status payment_generation_current payment_address order_request order_button
      frame stream sign device_pilot_button
  DEPENDS ${P01}/test/extract_c.py ${P01}/app/src/device_install.c ${P01}/app/src/device_pilot.c VERBATIM)

add_executable(order-board board_host.c ${EXTRACTED}
  ${P01}/app/src/device_order.c ${P01}/app/src/device_spending.c ${P01}/app/src/device_vault.c
  ${P01}/core/src/nu54_order.c ${P01}/core/src/nu54_spending.c ${P01}/core/src/nu54_pilot.c
  ${P01}/core/src/nu54_local_approval.c ${P01}/core/src/nu54_owner.c ${P01}/core/src/nu54_vault.c
  ${P01}/core/src/nu54_install.c ${P01}/core/src/nu54_running_firmware.c ${P01}/core/src/nu54_firmware_power.c
  ${P01}/core/src/nu54_keccak.c ${P01}/core/src/nu54_sig.c ${P01}/third_party/keccak/Keccak-readable-and-compact.c
  ${PROTOCOL}/src/nu54_quote_v2.c ${PROTOCOL}/src/nu54_transport_v2.c
  ${SECP}/src/secp256k1.c ${SECP}/src/precomputed_ecmult.c ${SECP}/src/precomputed_ecmult_gen.c)
target_include_directories(order-board BEFORE PRIVATE ${P01}/test/fakes-spending ${CMAKE_CURRENT_BINARY_DIR}
  ${P01}/app/src ${P01}/core/include ${PROTOCOL}/include ${SECP}/include)
target_compile_definitions(order-board PRIVATE ENABLE_MODULE_RECOVERY=1 ECMULT_WINDOW_SIZE=4
  CONFIG_NU54_SPENDING_PROFILE=1 CONFIG_NU54_SPENDING_DAILY_CAP=100000000)
set_source_files_properties(board_host.c PROPERTIES COMPILE_OPTIONS "-Wall;-Wextra;-Werror")
target_link_libraries(order-board PRIVATE OpenSSL::Crypto)

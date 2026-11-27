# The spending wallet firmware compiled for the host (board_host.c). Built by scripts/test_order_flow.py, and by the
# firmware host tests (products/p01-device-firmware/test/CMakeLists.txt), which need no database and no chain.
cmake_minimum_required(VERSION 3.20)
project(nu54_order_flow_board C)

find_package(Python3 REQUIRED COMPONENTS Interpreter)
find_package(OpenSSL REQUIRED)
get_filename_component(REPO ${CMAKE_CURRENT_SOURCE_DIR}/../../../../.. ABSOLUTE)
set(P01 ${REPO}/products/p01-device-firmware)
set(PROTOCOL ${REPO}/packages/protocol/c)
set(SECP ${P01}/third_party/secp256k1)

# A board for another token is made of copies of the sources that carry its name, and its unit when it has one,
# in the build directory.
set(NU54_TOKEN_NAME "NU54 Test USD" CACHE STRING "The token name this board signs for")
set(NU54_TOKEN_UNIT "" CACHE STRING "Base units of the token in one unit this board counts; empty when they are the same")
set(COPIES)
function(for_token source out)
  get_filename_component(name ${source} NAME)
  set(copy ${CMAKE_CURRENT_BINARY_DIR}/renamed/${name})
  file(MAKE_DIRECTORY ${CMAKE_CURRENT_BINARY_DIR}/renamed)
  add_custom_command(OUTPUT ${copy}
    COMMAND ${Python3_EXECUTABLE} -B ${CMAKE_CURRENT_SOURCE_DIR}/rename_token.py ${source} ${copy} ${NU54_TOKEN_NAME} ${NU54_TOKEN_UNIT}
    DEPENDS ${CMAKE_CURRENT_SOURCE_DIR}/rename_token.py ${source} VERBATIM)
  set(${out} ${copy} PARENT_SCOPE)
  set(COPIES ${COPIES} ${copy} PARENT_SCOPE)
endfunction()
set(ORDER_SOURCE ${P01}/core/src/nu54_order.c)
set(SPENDING_SOURCE ${P01}/core/src/nu54_spending.c)
set(PILOT_SOURCE ${P01}/core/src/nu54_pilot.c)
set(DEVICE_PILOT ${P01}/app/src/device_pilot.c)
set(ORDER_TEST ${P01}/test/test_order.c)
if(NOT NU54_TOKEN_NAME STREQUAL "NU54 Test USD")
  for_token(${ORDER_SOURCE} ORDER_SOURCE)
  for_token(${SPENDING_SOURCE} SPENDING_SOURCE)
endif()
if(NU54_TOKEN_UNIT)
  for_token(${PILOT_SOURCE} PILOT_SOURCE)
  for_token(${DEVICE_PILOT} DEVICE_PILOT)
  for_token(${ORDER_TEST} ORDER_TEST)
endif()
set(SIGNING ${ORDER_SOURCE} ${SPENDING_SOURCE})
if(COPIES)
  # Three programs compile the copies; one step makes them.
  add_custom_target(renamed DEPENDS ${COPIES})
endif()

# The message handling is taken out of the app sources, so this board runs the text the real one builds.
# So is the keeping of the grant and of the time in storage, and the clock the owner's phone sets.
set(EXTRACTED ${CMAKE_CURRENT_BINARY_DIR}/board_pilot.h)
add_custom_command(OUTPUT ${EXTRACTED}
  COMMAND ${Python3_EXECUTABLE} -B ${P01}/test/extract_c.py ${EXTRACTED}
    ${P01}/app/src/device_install.c OFFER_TTL_MS INSTALL_GRANT INSTALL_CLOCK INSTALL_GRANT_PART GRANT_PART_SIZE GRANT_PARTS
      CLOCK_SAVE_SECONDS CLOCK_SESSION_SECONDS CLOCK_FREE_SECONDS import_public verify device_install_verify_authority storage_found removed
      clock_load clock_store
      grant_store grant_read device_install_forget device_install_earliest no_owner hold_after device_install_keep_time
      device_install_set_time device_install_order_hold device_install_now device_install_policy
    ${DEVICE_PILOT} RESET_NONCE_MS reset_display_input reset_install_input reset_link
      session_live owner_pin_live pilot_pin send_frame present load_refusal clear_legacy_bonds report_legacy_bonds
      owner_absent reset_nonce_issue journal_new_session journal_follow_time reset_of_released spending_request
      spending_erase_and_reboot
      spending_button manage load_receipt send_receipt
      reclaim_receipt receipt_status payment_generation_current payment_address order_request order_button
      clock_teller clock_take clock_session_goes_on clock_request clock_wait_over clock_button frame stream sign
      bind_session device_pilot_button device_pilot_owner_absent keep_time
      resume_order_hold
  DEPENDS ${P01}/test/extract_c.py ${P01}/app/src/device_install.c ${DEVICE_PILOT} VERBATIM)

add_executable(order-board board_host.c ${EXTRACTED}
  ${P01}/app/src/device_order.c ${P01}/app/src/device_spending.c ${P01}/app/src/device_vault.c
  ${SIGNING} ${PILOT_SOURCE}
  ${P01}/core/src/nu54_local_approval.c ${P01}/core/src/nu54_owner.c ${P01}/core/src/nu54_vault.c
  ${P01}/core/src/nu54_install.c ${P01}/core/src/nu54_running_firmware.c ${P01}/core/src/nu54_firmware_power.c
  ${P01}/core/src/nu54_keccak.c ${P01}/core/src/nu54_sig.c ${P01}/third_party/keccak/Keccak-readable-and-compact.c
  ${PROTOCOL}/src/nu54_quote_v2.c ${PROTOCOL}/src/nu54_transport_v2.c
  ${SECP}/src/secp256k1.c ${SECP}/src/precomputed_ecmult.c ${SECP}/src/precomputed_ecmult_gen.c)
target_include_directories(order-board BEFORE PRIVATE ${P01}/test/fakes-spending ${CMAKE_CURRENT_BINARY_DIR} ${CMAKE_CURRENT_SOURCE_DIR}
  ${P01}/app/src ${P01}/core/include ${PROTOCOL}/include ${SECP}/include)
# One board stands in for every customer of the run, thirty of them in one flow, so its day allows 250 orders.
# It waits for its owner as the commercial board does, and saves its time as often (the Kconfig default).
target_compile_definitions(order-board PRIVATE ENABLE_MODULE_RECOVERY=1 ECMULT_WINDOW_SIZE=4
  CONFIG_NU54_SPENDING_PROFILE=1 CONFIG_NU54_SPENDING_DAILY_CAP=2000000000
  CONFIG_NU54_OWNER_WAIT_ADV=1 CONFIG_NU54_CLOCK_SAVE_SECONDS=3600 CONFIG_NU54_CLOCK_SESSION_SECONDS=15
  CONFIG_NU54_CLOCK_FREE_SECONDS=86400)
set_source_files_properties(board_host.c return_vector.c PROPERTIES COMPILE_OPTIONS "-Wall;-Wextra;-Werror")
target_link_libraries(order-board PRIVATE OpenSSL::Crypto)

# The firmware's own payment test, on the sources this board is built from.
enable_testing()
add_executable(order-vector ${ORDER_TEST} ${ORDER_SOURCE} ${PILOT_SOURCE}
  ${P01}/core/src/nu54_local_approval.c ${P01}/core/src/nu54_keccak.c ${P01}/core/src/nu54_sig.c
  ${P01}/third_party/keccak/Keccak-readable-and-compact.c ${PROTOCOL}/src/nu54_quote_v2.c ${PROTOCOL}/src/nu54_transport_v2.c
  ${SECP}/src/secp256k1.c ${SECP}/src/precomputed_ecmult.c ${SECP}/src/precomputed_ecmult_gen.c)
target_include_directories(order-vector PRIVATE ${CMAKE_CURRENT_SOURCE_DIR} ${P01}/core/include ${PROTOCOL}/include ${SECP}/include)
target_compile_definitions(order-vector PRIVATE ENABLE_MODULE_RECOVERY=1 ECMULT_WINDOW_SIZE=4)
add_executable(return-vector return_vector.c ${SIGNING} ${PILOT_SOURCE} ${P01}/core/src/nu54_button.c
  ${P01}/core/src/nu54_local_approval.c ${P01}/core/src/nu54_keccak.c ${P01}/core/src/nu54_sig.c
  ${P01}/third_party/keccak/Keccak-readable-and-compact.c ${PROTOCOL}/src/nu54_quote_v2.c ${PROTOCOL}/src/nu54_transport_v2.c
  ${SECP}/src/secp256k1.c ${SECP}/src/precomputed_ecmult.c ${SECP}/src/precomputed_ecmult_gen.c)
target_include_directories(return-vector PRIVATE ${CMAKE_CURRENT_SOURCE_DIR} ${P01}/core/include ${PROTOCOL}/include ${SECP}/include)
target_compile_definitions(return-vector PRIVATE ENABLE_MODULE_RECOVERY=1 ECMULT_WINDOW_SIZE=4 CONFIG_NU54_SPENDING_PROFILE=1)
set(NU54_ORDER_VECTOR ${REPO}/packages/protocol/v2/testdata/order-lock.vectors CACHE FILEPATH "Payment vector of the token")
set(NU54_RETURN_VECTOR ${REPO}/packages/protocol/v2/testdata/wallet-return.vectors CACHE FILEPATH "Return vector of the token")
set(NU54_RETURN_AMOUNT 9500000 CACHE STRING "Amount in the return vector")
if(NU54_TOKEN_UNIT)
  foreach(program order-board order-vector return-vector)
    target_compile_definitions(${program} PRIVATE NU54_TOKEN_UNIT=UINT64_C\(${NU54_TOKEN_UNIT}\))
  endforeach()
endif()
if(TARGET renamed)
  foreach(program order-board order-vector return-vector)
    add_dependencies(${program} renamed)
  endforeach()
endif()
# test_order.c checks with assert, so it must not be built without it.
target_compile_options(order-vector PRIVATE -UNDEBUG)
add_test(NAME order_vector COMMAND order-vector ${NU54_ORDER_VECTOR})
add_executable(unit-test unit_test.c)
target_compile_definitions(unit-test PRIVATE NU54_TOKEN_UNIT=UINT64_C\(10000000000000000\))
target_compile_options(unit-test PRIVATE -Wall -Wextra -Werror)
add_test(NAME token_unit COMMAND unit-test)
add_test(NAME return_vector COMMAND return-vector ${NU54_RETURN_VECTOR} ${NU54_RETURN_AMOUNT})
# The board itself, without an owner: no wallet key, the grant kept through a restart, a grant that ended.
add_test(NAME board_restart COMMAND ${Python3_EXECUTABLE} -B ${CMAKE_CURRENT_SOURCE_DIR}/restart_test.py $<TARGET_FILE:order-board>)

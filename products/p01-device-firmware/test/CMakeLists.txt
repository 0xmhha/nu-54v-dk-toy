# Host unit tests for the board-independent core (no Zephyr needed).
cmake_minimum_required(VERSION 3.20)
project(nu54_core_tests C)
enable_testing()

add_executable(test_frame test_frame.c ../core/src/nu54_frame.c)
target_include_directories(test_frame PRIVATE ../core/include)
target_compile_options(test_frame PRIVATE -Wall -Wextra -Werror)
add_test(NAME frame COMMAND test_frame)

add_executable(test_button test_button.c ../core/src/nu54_button.c)
target_include_directories(test_button PRIVATE ../core/include)
target_compile_options(test_button PRIVATE -Wall -Wextra -Werror)
add_test(NAME button COMMAND test_button)

add_executable(test_pin_entry test_pin_entry.c ../core/src/nu54_pin_entry.c)
target_include_directories(test_pin_entry PRIVATE ../core/include)
target_compile_options(test_pin_entry PRIVATE -Wall -Wextra -Werror)
add_test(NAME pin_entry COMMAND test_pin_entry)

add_executable(test_links test_links.c ../core/src/nu54_links.c)
target_include_directories(test_links PRIVATE ../core/include ../../../packages/protocol/c/include)
target_compile_options(test_links PRIVATE -Wall -Wextra -Werror)
add_test(NAME links COMMAND test_links)

add_executable(test_status_led test_status_led.c ../app/src/status_led.c)
target_include_directories(test_status_led PRIVATE ../core/include ../app/src fakes-status-led)
target_compile_options(test_status_led PRIVATE -Wall -Wextra -Werror)
add_test(NAME status_led COMMAND test_status_led)

# --- Cryptography (third_party) and shared-vector tests -------------------------------------
# libsecp256k1 v0.8.0 with the recovery module. ECMULT_WINDOW_SIZE 4 keeps the verification
# table small enough for the device; the host tests use the same setting.
find_package(Python3 REQUIRED COMPONENTS Interpreter)
set(SECP ${CMAKE_CURRENT_SOURCE_DIR}/../third_party/secp256k1)
add_library(nu54_secp256k1 STATIC ${SECP}/src/secp256k1.c ${SECP}/src/precomputed_ecmult.c ${SECP}/src/precomputed_ecmult_gen.c)
target_include_directories(nu54_secp256k1 PUBLIC ${SECP}/include PRIVATE ${SECP}/src)
target_compile_definitions(nu54_secp256k1 PRIVATE ENABLE_MODULE_RECOVERY=1 ENABLE_MODULE_ECDH=1 ECMULT_WINDOW_SIZE=4)

add_library(nu54_crypto STATIC
  ../core/src/nu54_keccak.c ../core/src/nu54_eip712.c ../core/src/nu54_sig.c ../core/src/nu54_secure.c
  ../third_party/keccak/Keccak-readable-and-compact.c)
target_include_directories(nu54_crypto PUBLIC ../core/include ../../../packages/protocol/c/include)
target_link_libraries(nu54_crypto PUBLIC nu54_secp256k1)
set_source_files_properties(../core/src/nu54_keccak.c ../core/src/nu54_eip712.c ../core/src/nu54_sig.c ../core/src/nu54_secure.c
  PROPERTIES COMPILE_OPTIONS "-Wall;-Wextra;-Werror")

# The shared vectors become a header in the build directory (never committed).
set(VECTORS_H ${CMAKE_CURRENT_BINARY_DIR}/vectors.h)
set(VECTOR_JSON
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../docs/content/specifications/protocol/eip712-vectors.json
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../docs/content/specifications/protocol/cbor-vectors.json
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../docs/content/specifications/protocol/frame-vectors.json
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../docs/content/specifications/protocol/session-vectors.json)
add_custom_command(OUTPUT ${VECTORS_H}
  COMMAND ${Python3_EXECUTABLE} ${CMAKE_CURRENT_SOURCE_DIR}/gen_vectors.py ${VECTORS_H}
  DEPENDS ${CMAKE_CURRENT_SOURCE_DIR}/gen_vectors.py ${VECTOR_JSON})
add_custom_target(nu54_vectors DEPENDS ${VECTORS_H})

add_executable(test_eip712 test_eip712.c)
target_include_directories(test_eip712 PRIVATE ${CMAKE_CURRENT_BINARY_DIR})
target_link_libraries(test_eip712 PRIVATE nu54_crypto)
target_compile_options(test_eip712 PRIVATE -Wall -Wextra -Werror)
add_dependencies(test_eip712 nu54_vectors)
add_test(NAME eip712_vectors COMMAND test_eip712)

add_executable(test_frame_vectors test_frame_vectors.c ../core/src/nu54_frame.c)
target_include_directories(test_frame_vectors PRIVATE ../core/include ${CMAKE_CURRENT_BINARY_DIR})
target_compile_options(test_frame_vectors PRIVATE -Wall -Wextra -Werror)
add_dependencies(test_frame_vectors nu54_vectors)
add_test(NAME frame_vectors COMMAND test_frame_vectors)

add_executable(test_cbor test_cbor.c ../core/src/nu54_cbor.c)
target_include_directories(test_cbor PRIVATE ../core/include ../../../packages/protocol/c/include ${CMAKE_CURRENT_BINARY_DIR})
target_compile_options(test_cbor PRIVATE -Wall -Wextra -Werror)
add_dependencies(test_cbor nu54_vectors)
add_test(NAME cbor_vectors COMMAND test_cbor)

# The session test plays the platform's HKDF and AES-GCM with OpenSSL (on the board PSA does it).
if(APPLE AND NOT OPENSSL_ROOT_DIR)
  execute_process(COMMAND brew --prefix openssl@3 OUTPUT_VARIABLE OPENSSL_ROOT_DIR OUTPUT_STRIP_TRAILING_WHITESPACE ERROR_QUIET)
endif()
find_package(OpenSSL REQUIRED COMPONENTS Crypto)
add_executable(test_session test_session.c ../core/src/nu54_session.c ../core/src/nu54_session_util.c
  ../core/src/nu54_session_channel.c ../core/src/nu54_session_setup.c ../core/src/nu54_session_pay.c
  ../core/src/nu54_session_limit.c ../core/src/nu54_cbor.c ../core/src/nu54_rental_journal.c ../core/src/nu54_rental_freeze.c ../core/src/nu54_rental_credential.c ../core/src/nu54_rental_retirement.c)
target_compile_definitions(test_session PRIVATE NU54_TEST_RENTAL_CLEANUP=1)
target_include_directories(test_session PRIVATE ${CMAKE_CURRENT_BINARY_DIR})
find_package(Threads REQUIRED)
target_link_libraries(test_session PRIVATE nu54_crypto OpenSSL::Crypto Threads::Threads)
target_compile_options(test_session PRIVATE -Wall -Wextra -Werror)
add_dependencies(test_session nu54_vectors)
add_test(NAME session_vectors COMMAND test_session
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../packages/protocol/v2/testdata/rjp1.vectors
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../packages/protocol/v2/testdata/rfr1.vectors
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../packages/protocol/v2/testdata/rfq2.vectors)

add_executable(test_rental_journal test_rental_journal.c ../core/src/nu54_rental_journal.c
  ../core/src/nu54_rental_retirement.c)
target_link_libraries(test_rental_journal PRIVATE nu54_crypto)
target_compile_options(test_rental_journal PRIVATE -Wall -Wextra -Werror)
add_test(NAME rental_journal_durability COMMAND test_rental_journal
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../packages/protocol/v2/testdata/rjp1.vectors)

add_executable(test_rental_cleanup test_rental_cleanup.c ../core/src/nu54_rental_retirement.c ../core/src/nu54_rental_journal.c)
target_link_libraries(test_rental_cleanup PRIVATE nu54_crypto)
target_compile_options(test_rental_cleanup PRIVATE -Wall -Wextra -Werror)
add_test(NAME rental_cleanup_proof COMMAND test_rental_cleanup
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../packages/protocol/v2/testdata/rental-cleanup.vectors)

add_executable(test_rental_journal_ps test_rental_journal_ps.c
  ../app/src/device_rental_journal_ps.c ../core/src/nu54_rental_registration.c ../core/src/nu54_rental_journal.c
  ../core/src/nu54_rental_retirement.c)
target_include_directories(test_rental_journal_ps PRIVATE ../app/src fakes-owner-limits)
target_compile_definitions(test_rental_journal_ps PRIVATE CONFIG_TRUSTED_EXECUTION_NONSECURE
  CONFIG_TFM_PARTITION_PROTECTED_STORAGE CONFIG_TFM_PS_ROLLBACK_PROTECTION CONFIG_TFM_PS_MAX_ASSET_SIZE=2048)
target_link_libraries(test_rental_journal_ps PRIVATE nu54_crypto)
target_compile_options(test_rental_journal_ps PRIVATE -Wall -Wextra -Werror)
add_test(NAME rental_journal_protected_storage COMMAND test_rental_journal_ps)

add_executable(test_vault test_vault.c ../core/src/nu54_vault.c)
target_include_directories(test_vault PRIVATE ../core/include)
target_compile_options(test_vault PRIVATE -Wall -Wextra -Werror)
add_test(NAME vault_interruption COMMAND test_vault)

add_executable(test_device_key test_device_key.c ../app/src/device_key.c)
target_include_directories(test_device_key PRIVATE fakes ../app/src)
target_link_libraries(test_device_key PRIVATE nu54_crypto)
target_compile_options(test_device_key PRIVATE -Wall -Wextra -Werror)
add_test(NAME device_key_slots COMMAND test_device_key)

add_executable(test_pin_attempt test_pin_attempt.c ../core/src/nu54_pin_attempt.c)
target_include_directories(test_pin_attempt PRIVATE ../core/include)
target_compile_options(test_pin_attempt PRIVATE -Wall -Wextra -Werror)
add_test(NAME pin_attempt_durability COMMAND test_pin_attempt)

add_executable(test_local_approval test_local_approval.c ../core/src/nu54_local_approval.c)
target_include_directories(test_local_approval PRIVATE ../core/include)
target_compile_options(test_local_approval PRIVATE -Wall -Wextra -Werror)
add_test(NAME local_approval_binding COMMAND test_local_approval)

add_executable(test_owner test_owner.c ../core/src/nu54_owner.c ../core/src/nu54_vault.c)
target_include_directories(test_owner PRIVATE ../core/include ../../../packages/protocol/c/include)
target_link_libraries(test_owner PRIVATE OpenSSL::Crypto)
target_compile_options(test_owner PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_physical_enrollment COMMAND test_owner)

add_executable(test_hd test_hd.c ../core/src/nu54_accessory.c ../core/src/nu54_hd.c ../core/src/nu54_hd_slot.c ../core/src/nu54_recovery.c)
target_link_libraries(test_hd PRIVATE nu54_crypto OpenSSL::Crypto)
target_compile_options(test_hd PRIVATE -Wall -Wextra -Werror)
add_test(NAME hd_recovery_two_stores COMMAND test_hd)

find_package(OpenSSL REQUIRED COMPONENTS SSL)
add_executable(recovery-accessory ../recovery-accessory/main.c ../core/src/nu54_accessory.c ../core/src/nu54_hd.c ../core/src/nu54_recovery.c)
target_link_libraries(recovery-accessory PRIVATE nu54_crypto OpenSSL::SSL)
target_compile_options(recovery-accessory PRIVATE -Wall -Wextra -Werror)
add_test(NAME offline_recovery_accessory_transport
  COMMAND ${Python3_EXECUTABLE} ${CMAKE_CURRENT_SOURCE_DIR}/test_accessory_transport.py $<TARGET_FILE:recovery-accessory>)
set_tests_properties(offline_recovery_accessory_transport PROPERTIES TIMEOUT 45)

add_executable(test_install test_install.c ../core/src/nu54_install.c ../core/src/nu54_pilot.c ../core/src/nu54_hd.c
  ../core/src/nu54_local_approval.c ../../../packages/protocol/c/src/nu54_quote_v2.c
  ../../../packages/protocol/c/src/nu54_transport_v2.c)
target_link_libraries(test_install PRIVATE nu54_crypto OpenSSL::Crypto)
target_compile_options(test_install PRIVATE -Wall -Wextra -Werror)
add_test(NAME enrollment_policy COMMAND test_install)

add_executable(test_order test_order.c ../core/src/nu54_order.c ../core/src/nu54_pilot.c
  ../core/src/nu54_local_approval.c ../../../packages/protocol/c/src/nu54_quote_v2.c)
target_link_libraries(test_order PRIVATE nu54_crypto)
target_compile_options(test_order PRIVATE -Wall -Wextra -Werror)
add_test(NAME order_payment COMMAND test_order
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../packages/protocol/v2/testdata/order-lock.vectors)

add_executable(test_spending test_spending.c ../core/src/nu54_spending.c ../core/src/nu54_button.c
  ../core/src/nu54_pin_attempt.c)
target_link_libraries(test_spending PRIVATE nu54_crypto)
target_compile_options(test_spending PRIVATE -Wall -Wextra -Werror)
add_test(NAME spending_actions COMMAND test_spending
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../packages/protocol/v2/testdata/wallet-return.vectors)

add_executable(test_spending_store test_spending_store.c ../app/src/device_spending.c ../app/src/device_order.c
  ../core/src/nu54_spending.c ../core/src/nu54_order.c ../core/src/nu54_pilot.c
  ../core/src/nu54_local_approval.c ../core/src/nu54_vault.c ../core/src/nu54_pin_attempt.c
  ../core/src/nu54_pin_entry.c ../../../packages/protocol/c/src/nu54_quote_v2.c)
target_include_directories(test_spending_store BEFORE PRIVATE fakes-spending ../app/src)
target_compile_definitions(test_spending_store PRIVATE CONFIG_NU54_SPENDING_PROFILE=1 CONFIG_NU54_SPENDING_DAILY_CAP=100)
target_link_libraries(test_spending_store PRIVATE nu54_crypto)
target_compile_options(test_spending_store PRIVATE -Wall -Wextra -Werror)
add_test(NAME spending_store COMMAND test_spending_store)

add_custom_command(OUTPUT ${CMAKE_CURRENT_BINARY_DIR}/board_buttons.h
  COMMAND ${Python3_EXECUTABLE} ${CMAKE_CURRENT_SOURCE_DIR}/extract_c.py ${CMAKE_CURRENT_BINARY_DIR}/board_buttons.h
    ${CMAKE_CURRENT_SOURCE_DIR}/../app/src/device_pilot.c owner_absent reset_of_released pilot_pin spending_erase_and_reboot spending_button
    order_button device_pilot_button_reset device_pilot_button device_pilot_engaged
  DEPENDS extract_c.py ../app/src/device_pilot.c VERBATIM)
add_executable(test_spending_led test_spending_led.c ../app/src/status_led.c ../core/src/nu54_spending.c
  ../core/src/nu54_adv_schedule.c ${CMAKE_CURRENT_BINARY_DIR}/board_buttons.h)
target_include_directories(test_spending_led PRIVATE ../app/src fakes-status-led ${CMAKE_CURRENT_BINARY_DIR})
target_compile_definitions(test_spending_led PRIVATE CONFIG_NU54_SPENDING_PROFILE=1)
target_link_libraries(test_spending_led PRIVATE nu54_crypto)
target_compile_options(test_spending_led PRIVATE -Wall -Wextra -Werror)
add_test(NAME spending_led COMMAND test_spending_led)

add_custom_command(OUTPUT ${CMAKE_CURRENT_BINARY_DIR}/board_journal.h
  COMMAND ${Python3_EXECUTABLE} ${CMAKE_CURRENT_SOURCE_DIR}/extract_c.py ${CMAKE_CURRENT_BINARY_DIR}/board_journal.h
    ${CMAKE_CURRENT_SOURCE_DIR}/../app/src/device_pilot.c journal_slots journal_new_session
  DEPENDS extract_c.py ../app/src/device_pilot.c VERBATIM)
add_custom_target(board_journal DEPENDS ${CMAKE_CURRENT_BINARY_DIR}/board_journal.h)
foreach(profile pilot spending)
  add_executable(test_owner_journal_${profile} test_owner_journal.c ../../../packages/protocol/c/src/nu54_transport_v2.c)
  target_include_directories(test_owner_journal_${profile} PRIVATE ../core/include ../../../packages/protocol/c/include
    ${CMAKE_CURRENT_BINARY_DIR})
  target_compile_options(test_owner_journal_${profile} PRIVATE -Wall -Wextra -Werror)
  add_dependencies(test_owner_journal_${profile} board_journal)
  add_test(NAME owner_journal_${profile} COMMAND test_owner_journal_${profile})
endforeach()
target_compile_definitions(test_owner_journal_spending PRIVATE CONFIG_NU54_SPENDING_PROFILE=1)
target_compile_options(test_owner_journal_pilot PRIVATE -Wno-unused-function)

# The operator's side of the spending profile: enroll_board.py against the board's own checks.
add_custom_command(OUTPUT ${CMAKE_CURRENT_BINARY_DIR}/board_authority_check.h
  COMMAND ${Python3_EXECUTABLE} ${CMAKE_CURRENT_SOURCE_DIR}/extract_c.py ${CMAKE_CURRENT_BINARY_DIR}/board_authority_check.h
    ${CMAKE_CURRENT_SOURCE_DIR}/../app/src/device_install.c hash import_public verify device_install_verify_authority
    ${CMAKE_CURRENT_SOURCE_DIR}/../app/src/device_pilot.c RESET_NONCE_MS reset_nonce_issue reset_of_released
    spending_request
  DEPENDS extract_c.py ../app/src/device_install.c ../app/src/device_pilot.c VERBATIM)
add_executable(spending_operator_host spending_operator_host.c ${CMAKE_CURRENT_BINARY_DIR}/board_authority_check.h
  ../core/src/nu54_spending.c ../core/src/nu54_install.c ../core/src/nu54_owner.c ../core/src/nu54_vault.c
  ../core/src/nu54_pilot.c ../core/src/nu54_local_approval.c ../../../packages/protocol/c/src/nu54_quote_v2.c
  ../../../packages/protocol/c/src/nu54_transport_v2.c)
target_include_directories(spending_operator_host PRIVATE ${CMAKE_CURRENT_BINARY_DIR})
target_link_libraries(spending_operator_host PRIVATE nu54_crypto OpenSSL::Crypto)
target_compile_options(spending_operator_host PRIVATE -Wall -Wextra -Werror)

add_executable(test_evm test_evm.c ../core/src/nu54_evm_tx.c ../core/src/nu54_siwe.c)
target_link_libraries(test_evm PRIVATE nu54_crypto)
target_compile_options(test_evm PRIVATE -Wall -Wextra -Werror)

# Optional real-node harness uses the exact portable firmware transaction code.
# run_wallet_chain_nodes.py starts isolated validator processes and binds loopback.
add_library(wallet_chain_node_oracle MODULE chain_node_oracle.c
  ../core/src/nu54_hd.c ../core/src/nu54_btc.c ../core/src/nu54_btc48.c ../core/src/nu54_solana.c ../core/src/nu54_xrpl.c)
target_link_libraries(wallet_chain_node_oracle PRIVATE nu54_crypto OpenSSL::Crypto)
set_target_properties(nu54_crypto nu54_secp256k1 PROPERTIES POSITION_INDEPENDENT_CODE ON)
add_test(NAME bip48_registered_threshold
  COMMAND ${Python3_EXECUTABLE} ${CMAKE_CURRENT_SOURCE_DIR}/test_bip48.py $<TARGET_FILE:wallet_chain_node_oracle>)

add_executable(test_device_evm test_device_evm.c ../app/src/device_evm.c ../core/src/nu54_market_lab.c ../core/src/nu54_evm_tx.c ../core/src/nu54_siwe.c)
target_include_directories(test_device_evm PRIVATE fakes ../app/src)
target_link_libraries(test_device_evm PRIVATE nu54_crypto)
target_compile_options(test_device_evm PRIVATE -Wall -Wextra -Werror)

# W27 policy transition model only; hardware activation requires a separate reviewed integration.
add_executable(test_recovery_pin_guard test_recovery_pin_guard.c ../app/src/device_recovery_pin.c
    ../core/src/nu54_pin_entry.c ../core/src/nu54_pin_attempt.c)
target_include_directories(test_recovery_pin_guard PRIVATE fakes-owner-idle ../app/src ../core/include ../../../packages/protocol/c/include)
target_compile_options(test_recovery_pin_guard PRIVATE -Wall -Wextra -Werror)
add_test(NAME recovery_pin_guard COMMAND test_recovery_pin_guard)

add_executable(test_owner_idle test_owner_idle.c)
target_include_directories(test_owner_idle PRIVATE ../core/include)
target_compile_options(test_owner_idle PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_idle COMMAND test_owner_idle)

add_executable(test_owner_limits test_owner_limits.c ../core/src/nu54_owner_limits.c)
target_include_directories(test_owner_limits PRIVATE ../core/include)
target_compile_options(test_owner_limits PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_limits COMMAND test_owner_limits)

add_executable(test_owner_limits_store test_owner_limits_store.c ../core/src/nu54_owner_limits.c ../core/src/nu54_owner_limits_store.c)
target_include_directories(test_owner_limits_store PRIVATE ../core/include)
target_compile_options(test_owner_limits_store PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_limits_store COMMAND test_owner_limits_store)
add_executable(test_owner_limits_ps test_owner_limits_ps.c ../app/src/device_owner_limits_ps.c ../core/src/nu54_owner_limits.c ../core/src/nu54_owner_limit_change.c ../core/src/nu54_local_approval.c)
target_include_directories(test_owner_limits_ps PRIVATE fakes-owner-limits ../app/src ../core/include ../../../packages/protocol/c/include)
target_compile_definitions(test_owner_limits_ps PRIVATE CONFIG_TRUSTED_EXECUTION_NONSECURE CONFIG_TFM_PARTITION_PROTECTED_STORAGE CONFIG_TFM_PS_ROLLBACK_PROTECTION)
target_compile_options(test_owner_limits_ps PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_limits_ps COMMAND test_owner_limits_ps)
add_executable(test_owner_limit_change test_owner_limit_change.c ../core/src/nu54_owner_limit_change.c ../core/src/nu54_owner_limits.c ../core/src/nu54_local_approval.c)
target_include_directories(test_owner_limit_change PRIVATE ../core/include)
target_compile_options(test_owner_limit_change PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_limit_change COMMAND test_owner_limit_change)
add_executable(test_owner_limits_evm_gate test_owner_limits_evm_gate.c ../app/src/device_evm.c ../core/src/nu54_market_lab.c ../core/src/nu54_evm_tx.c ../core/src/nu54_siwe.c)
target_include_directories(test_owner_limits_evm_gate PRIVATE fakes ../app/src)
target_compile_definitions(test_owner_limits_evm_gate PRIVATE CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
target_link_libraries(test_owner_limits_evm_gate PRIVATE nu54_crypto)
target_compile_options(test_owner_limits_evm_gate PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_limits_evm_gate COMMAND test_owner_limits_evm_gate)
add_executable(test_owner_limits_bootstrap test_owner_limits_bootstrap.c ../app/src/device_owner_limits_ps.c ../core/src/nu54_owner_limits.c ../core/src/nu54_owner_limit_change.c ../core/src/nu54_local_approval.c)
target_include_directories(test_owner_limits_bootstrap PRIVATE fakes-owner-limits ../app/src ../core/include ../../../packages/protocol/c/include)
target_compile_definitions(test_owner_limits_bootstrap PRIVATE CONFIG_TRUSTED_EXECUTION_NONSECURE CONFIG_TFM_PARTITION_PROTECTED_STORAGE CONFIG_TFM_PS_ROLLBACK_PROTECTION)
target_compile_options(test_owner_limits_bootstrap PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_limits_bootstrap COMMAND test_owner_limits_bootstrap)
add_executable(test_owner_limits_crypto test_owner_limits_crypto.c ../app/src/device_owner_limits_ps.c ../core/src/nu54_owner_limits.c ../core/src/nu54_owner_limit_change.c ../core/src/nu54_local_approval.c)
target_include_directories(test_owner_limits_crypto PRIVATE fakes-owner-limits ../app/src ../core/include ../../../packages/protocol/c/include)
target_compile_definitions(test_owner_limits_crypto PRIVATE CONFIG_TRUSTED_EXECUTION_NONSECURE CONFIG_TFM_PARTITION_PROTECTED_STORAGE CONFIG_TFM_PS_ROLLBACK_PROTECTION)
target_link_libraries(test_owner_limits_crypto PRIVATE OpenSSL::Crypto)
target_compile_options(test_owner_limits_crypto PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_limits_crypto COMMAND test_owner_limits_crypto)
add_executable(test_owner_limits_evm_ps test_owner_limits_evm_ps.c ../app/src/device_evm.c ../app/src/device_owner_limits_ps.c ../core/src/nu54_owner_limits.c ../core/src/nu54_owner_limit_change.c ../core/src/nu54_local_approval.c ../core/src/nu54_market_lab.c ../core/src/nu54_evm_tx.c ../core/src/nu54_siwe.c ../core/src/nu54_hd.c)
target_include_directories(test_owner_limits_evm_ps PRIVATE fakes-owner-limits fakes ../app/src ../core/include ../../../packages/protocol/c/include)
target_compile_definitions(test_owner_limits_evm_ps PRIVATE CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA CONFIG_TRUSTED_EXECUTION_NONSECURE CONFIG_TFM_PARTITION_PROTECTED_STORAGE CONFIG_TFM_PS_ROLLBACK_PROTECTION)
target_link_libraries(test_owner_limits_evm_ps PRIVATE nu54_crypto OpenSSL::Crypto)
target_compile_options(test_owner_limits_evm_ps PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_limits_evm_ps COMMAND test_owner_limits_evm_ps)
add_executable(owner_limits_host_bridge owner_limits_host_bridge.c ../app/src/device_owner_limits_ps.c ../app/src/device_evm.c ../core/src/nu54_owner_limits.c ../core/src/nu54_owner_limit_change.c ../core/src/nu54_local_approval.c ../core/src/nu54_market_lab.c ../core/src/nu54_evm_tx.c ../core/src/nu54_siwe.c)
target_include_directories(owner_limits_host_bridge PRIVATE fakes-owner-limits fakes ../app/src ../core/include ../../../packages/protocol/c/include)
target_compile_definitions(owner_limits_host_bridge PRIVATE CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA CONFIG_TRUSTED_EXECUTION_NONSECURE CONFIG_TFM_PARTITION_PROTECTED_STORAGE CONFIG_TFM_PS_ROLLBACK_PROTECTION)
target_link_libraries(owner_limits_host_bridge PRIVATE nu54_crypto OpenSSL::Crypto)
target_compile_options(owner_limits_host_bridge PRIVATE -Wall -Wextra -Werror)
add_executable(test_owner_finality test_owner_finality.c ../core/src/nu54_owner_finality_store.c ../core/src/nu54_owner_finality.c ../core/src/nu54_owner_limits.c ../core/src/nu54_evm_tx.c)
target_include_directories(test_owner_finality PRIVATE ../core/include)
target_link_libraries(test_owner_finality PRIVATE nu54_crypto OpenSSL::Crypto)
target_compile_options(test_owner_finality PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_finality COMMAND test_owner_finality)
add_executable(test_owner_finality_node test_owner_finality_node.c ../app/src/device_owner_finality_ps.c ../core/src/nu54_owner_finality_enroll.c ../core/src/nu54_local_approval.c ../core/src/nu54_owner_finality_store.c ../core/src/nu54_owner_finality.c ../core/src/nu54_owner_limits.c ../core/src/nu54_evm_tx.c)
target_include_directories(test_owner_finality_node PRIVATE fakes-owner-finality fakes-owner-limits ../app/src ../core/include ../../../packages/protocol/c/include)
target_compile_definitions(test_owner_finality_node PRIVATE CONFIG_TRUSTED_EXECUTION_NONSECURE CONFIG_TFM_PARTITION_PROTECTED_STORAGE CONFIG_TFM_PS_ROLLBACK_PROTECTION)
target_link_libraries(test_owner_finality_node PRIVATE nu54_crypto OpenSSL::Crypto)
target_compile_options(test_owner_finality_node PRIVATE -Wall -Wextra -Werror)
add_executable(test_owner_finality_ps test_owner_finality_ps.c ../core/src/nu54_owner_finality_store.c ../app/src/device_owner_finality_ps.c ../core/src/nu54_owner_finality_enroll.c ../core/src/nu54_local_approval.c ../core/src/nu54_owner_finality.c ../core/src/nu54_owner_limits.c ../core/src/nu54_evm_tx.c)
target_include_directories(test_owner_finality_ps PRIVATE fakes-owner-finality fakes-owner-limits ../app/src ../core/include ../../../packages/protocol/c/include)
target_compile_definitions(test_owner_finality_ps PRIVATE CONFIG_TRUSTED_EXECUTION_NONSECURE CONFIG_TFM_PARTITION_PROTECTED_STORAGE CONFIG_TFM_PS_ROLLBACK_PROTECTION)
target_link_libraries(test_owner_finality_ps PRIVATE nu54_crypto OpenSSL::Crypto)
target_compile_options(test_owner_finality_ps PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_finality_ps COMMAND test_owner_finality_ps)
add_executable(test_owner_finality_evm test_owner_finality_evm.c ../app/src/device_owner_finality_ps.c ../core/src/nu54_owner_finality_enroll.c ../app/src/device_evm.c ../core/src/nu54_owner_finality_store.c ../core/src/nu54_owner_finality.c ../core/src/nu54_owner_limits.c ../core/src/nu54_owner_limit_change.c ../core/src/nu54_local_approval.c ../core/src/nu54_market_lab.c ../core/src/nu54_evm_tx.c ../core/src/nu54_siwe.c)
target_include_directories(test_owner_finality_evm PRIVATE fakes-owner-finality fakes-owner-limits fakes ../app/src ../core/include ../../../packages/protocol/c/include)
target_compile_definitions(test_owner_finality_evm PRIVATE CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY CONFIG_TRUSTED_EXECUTION_NONSECURE CONFIG_TFM_PARTITION_PROTECTED_STORAGE CONFIG_TFM_PS_ROLLBACK_PROTECTION)
target_link_libraries(test_owner_finality_evm PRIVATE nu54_crypto OpenSSL::Crypto)
target_compile_options(test_owner_finality_evm PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_finality_evm COMMAND test_owner_finality_evm)
add_executable(test_owner_finality_enroll test_owner_finality_enroll.c ../core/src/nu54_owner_finality_enroll.c ../core/src/nu54_local_approval.c)
target_include_directories(test_owner_finality_enroll PRIVATE ../core/include)
target_compile_options(test_owner_finality_enroll PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_finality_enroll COMMAND test_owner_finality_enroll)
add_executable(test_owner_finality_migrate test_owner_finality_migrate.c ../core/src/nu54_owner_finality_migrate.c ../core/src/nu54_owner_finality_store.c ../core/src/nu54_owner_finality.c ../core/src/nu54_owner_limits.c ../core/src/nu54_evm_tx.c)
target_include_directories(test_owner_finality_migrate PRIVATE ../core/include)
target_link_libraries(test_owner_finality_migrate PRIVATE nu54_crypto OpenSSL::Crypto)
target_compile_options(test_owner_finality_migrate PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_finality_migrate COMMAND test_owner_finality_migrate)
add_executable(owner_core_host_bridge owner_core_host_bridge.c ../core/src/nu54_owner.c ../core/src/nu54_vault.c)
target_include_directories(owner_core_host_bridge PRIVATE ../core/include ../../../packages/protocol/c/include)
target_link_libraries(owner_core_host_bridge PRIVATE OpenSSL::Crypto)
target_compile_options(owner_core_host_bridge PRIVATE -Wall -Wextra -Werror)

add_executable(owner_registration_host owner_registration_host.c hd_accessory_store.c
    ../core/src/nu54_pilot.c ../core/src/nu54_local_approval.c
    ../core/src/nu54_running_firmware.c
    ../core/src/nu54_owner.c ../core/src/nu54_vault.c ../core/src/nu54_hd.c
    ../core/src/nu54_hd_slot.c ../core/src/nu54_accessory.c ../core/src/nu54_recovery.c
    ../../../packages/protocol/c/src/nu54_transport_v2.c
    ../../../packages/protocol/c/src/nu54_transport_v2_openssl.c
    ../../../packages/protocol/c/src/nu54_quote_v2.c)
target_include_directories(owner_registration_host PRIVATE ../core/include ../../../packages/protocol/c/include)
target_link_libraries(owner_registration_host PRIVATE nu54_crypto OpenSSL::SSL OpenSSL::Crypto)
target_compile_options(owner_registration_host PRIVATE -Wall -Wextra -Werror)

add_executable(hd_accessory_board hd_accessory_board.c hd_accessory_store.c
    ../core/src/nu54_accessory.c ../core/src/nu54_hd.c
    ../core/src/nu54_hd_slot.c ../core/src/nu54_recovery.c)
target_link_libraries(hd_accessory_board PRIVATE nu54_crypto OpenSSL::Crypto)
target_compile_options(hd_accessory_board PRIVATE -Wall -Wextra -Werror)
add_test(NAME hd_accessory_independent_backup
    COMMAND ${Python3_EXECUTABLE} ${CMAKE_CURRENT_SOURCE_DIR}/test_accessory_board.py
        $<TARGET_FILE:hd_accessory_board> $<TARGET_FILE:recovery-accessory>)

add_executable(test_w27_qualification test_w27_qualification.c)
target_include_directories(test_w27_qualification PRIVATE ../app/experimental/w27_qualification)
target_link_libraries(test_w27_qualification PRIVATE OpenSSL::Crypto)
target_compile_options(test_w27_qualification PRIVATE -Wall -Wextra -Werror)
add_test(NAME w27_qualification COMMAND test_w27_qualification)

add_executable(test_running_firmware test_running_firmware.c ../core/src/nu54_running_firmware.c)
target_include_directories(test_running_firmware PRIVATE ../core/include)
target_compile_options(test_running_firmware PRIVATE -Wall -Wextra -Werror)
add_test(NAME running_firmware_readonly COMMAND test_running_firmware)

add_executable(test_firmware_power test_firmware_power.c ../core/src/nu54_firmware_power.c)
target_include_directories(test_firmware_power PRIVATE ../core/include)
target_compile_options(test_firmware_power PRIVATE -Wall -Wextra -Werror)
add_test(NAME firmware_power_readonly COMMAND test_firmware_power)

# The low-power advertising schedule of a board that waits for its next owner.
add_executable(test_adv_schedule test_adv_schedule.c ../core/src/nu54_adv_schedule.c)
target_include_directories(test_adv_schedule PRIVATE ../core/include)
target_compile_options(test_adv_schedule PRIVATE -Wall -Wextra -Werror)
add_test(NAME adv_schedule COMMAND test_adv_schedule)

# The advertising and connection code of the board itself, over a fake controller.
add_custom_command(OUTPUT ${CMAKE_CURRENT_BINARY_DIR}/board_links.h
  COMMAND ${Python3_EXECUTABLE} ${CMAKE_CURRENT_SOURCE_DIR}/extract_c.py ${CMAKE_CURRENT_BINARY_DIR}/board_links.h
    ${CMAKE_CURRENT_SOURCE_DIR}/../app/src/ble_links.c set_mode owner_wait_phase start_set advertise_as advertise
    adv_retry owner_wait_turn idle_guard ble_links_owner_wait stop_advertising_if_idle mode_end
    ble_links_payment_mode connected
  DEPENDS extract_c.py ../app/src/ble_links.c VERBATIM)
add_executable(test_owner_wait_links test_owner_wait_links.c ../core/src/nu54_adv_schedule.c
  ${CMAKE_CURRENT_BINARY_DIR}/board_links.h)
target_include_directories(test_owner_wait_links PRIVATE ../core/include ${CMAKE_CURRENT_BINARY_DIR})
target_compile_options(test_owner_wait_links PRIVATE -Wall -Wextra -Werror)
add_test(NAME owner_wait_links COMMAND test_owner_wait_links)

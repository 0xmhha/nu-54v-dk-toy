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

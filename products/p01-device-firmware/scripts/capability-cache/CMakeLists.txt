function(nu54_test_capability_cache_PRE_IMAGE_CMAKE)
  cmake_parse_arguments(cache "" "IMAGE" "IMAGES" ${ARGN})
  # Zephyr selects its compiler cache before importing image-prefixed sysbuild options.
  # TF-M configures later in its own CMake process.
  set_property(TARGET "${cache_IMAGE}" APPEND PROPERTY _EP_CMAKE_ARGS
               "-DUSER_CACHE_DIR:PATH=${USER_CACHE_DIR}"
               "-DTFM_CMAKE_ARGS:STRING=-DUSER_CACHE_DIR:PATH=${USER_CACHE_DIR}")
endfunction()

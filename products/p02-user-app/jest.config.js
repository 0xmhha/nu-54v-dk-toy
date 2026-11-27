module.exports = {
  preset: '@react-native/jest-preset',
  setupFilesAfterEnv: ['<rootDir>/test/setup.js'],
  moduleNameMapper: { '^react$': require.resolve('react'), '^react/jsx-runtime$': require.resolve('react/jsx-runtime'), '^react/jsx-dev-runtime$': require.resolve('react/jsx-dev-runtime') },
};

const {approvedOrigin, inlineTravelOrigin} = require('./build-tools/public-travel-origin');

module.exports = api => {
  const origin = api.cache.using(() => approvedOrigin(process.env.NU54_P09_PUBLIC_ORIGIN));
  return {
    presets: ['module:@react-native/babel-preset'],
    plugins: [[inlineTravelOrigin, {origin}]],
  };
};

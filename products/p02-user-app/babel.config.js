const {approvedOrigin, inlineTravelOrigin} = require('./build-tools/public-travel-origin');
const {customerOrderBuild, inlineCustomerOrderBuild} = require('./build-tools/customer-order-build');

module.exports = api => {
  const origin = api.cache.using(() => approvedOrigin(process.env.NU54_P09_PUBLIC_ORIGIN));
  const build = JSON.parse(api.cache.using(() => JSON.stringify(customerOrderBuild(process.env.NU54_CUSTOMER_ORDER_BUILD))));
  return {
    presets: ['module:@react-native/babel-preset'],
    plugins: [[inlineTravelOrigin, {origin}], [inlineCustomerOrderBuild, {build}]],
  };
};

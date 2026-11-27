const path=require('node:path');
const config=require('../../metro.config.js');
const previous=config.resolver.resolveRequest;
const allowance=process.env.NU54_W15_QA==='1';
const discovery=process.env.NU54_W25_QA==='1';
const replacements={
  'NativeOwnerWallet.ts':'walletNativeOwner.js',
  'NativeRenterBle.ts':'walletNativeBle.js',
};
module.exports={
  ...config,
  cacheVersion:config.cacheVersion+':dapp-native-transport-qa:'+(allowance ? 'w15' : discovery ? 'w25' : 'default'),
  resolver:{...config.resolver,resolveRequest(context,moduleName,platform) {
    if ((allowance || discovery) && context.originModulePath===path.join(__dirname,'walletNativeOwner.js') &&
        moduleName==='./walletNativeEndpoint.js')
      return {type:'sourceFile',filePath:path.join(__dirname,allowance ? 'walletNativeAllowanceEndpoint.js' : 'walletNativeDiscoveryEndpoint.js')};
    const file=path.basename(moduleName);
    if (Object.hasOwn(replacements,file) && moduleName.includes('/specs/'))
      return {type:'sourceFile',filePath:path.join(__dirname,replacements[file])};
    return previous(context,moduleName,platform);
  }},
};

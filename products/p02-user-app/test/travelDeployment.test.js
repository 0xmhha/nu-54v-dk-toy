const {transformFileSync}=require('@babel/core');
const path=require('node:path');

test('places deployment is bound at bundle time and rejects insecure origins',()=>{
 const target=path.resolve(__dirname,'../src/travel/publicDeployment.ts');
 const compile=origin=>{
   const prior=process.env.NU54_P09_PUBLIC_ORIGIN;
   try{
     process.env.NU54_P09_PUBLIC_ORIGIN=origin;
     return transformFileSync(target,{configFile:path.resolve(__dirname,'../babel.config.js')}).code;
   }finally{
     if(prior===undefined) delete process.env.NU54_P09_PUBLIC_ORIGIN;
     else process.env.NU54_P09_PUBLIC_ORIGIN=prior;
   }
 };
 expect(compile('https://places.example')).toContain('https://places.example');
 expect(()=>compile('http://places.example')).toThrow('approved HTTPS public origin');
 expect(()=>compile('https://places.example/somepath')).toThrow('approved HTTPS public origin');
});

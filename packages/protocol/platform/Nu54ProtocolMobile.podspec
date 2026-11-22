require 'json'

package = JSON.parse(File.read(File.join(__dir__, 'package.json')))
Pod::Spec.new do |spec|
  spec.name = 'Nu54ProtocolMobile'
  spec.version = package.fetch('version')
  spec.summary = package.fetch('description')
  spec.homepage = 'https://github.com/0xmhha/nu-54v-dk-toy'
  spec.license = { :type => package.fetch('license'), :file => 'LICENSE' }
  spec.authors = 'NU54'
  spec.source = { :git => spec.homepage, :tag => "protocol-mobile-v#{spec.version}" }
  spec.ios.deployment_target = '15.1'
  # The final consumer compiles these headers as Objective-C++ with its own generated TurboModule specs.
  spec.source_files = 'ios/*.h'
  spec.public_header_files = 'ios/*.h'
  spec.pod_target_xcconfig = { 'DEFINES_MODULE' => 'NO' }
  spec.frameworks = 'CoreBluetooth', 'Network', 'Security'
end

# frozen_string_literal: true

require "digest"
require "active_support/inflector"

spec = Gem.loaded_specs.fetch("activesupport")
original = File.join(spec.full_gem_path, "lib/active_support/inflector/methods.rb")
backport = File.join(__dir__, "active_support_literal_match.rb")
original_digest = Digest::SHA256.file(original).hexdigest
backport_digest = Digest::SHA256.file(backport).hexdigest
unless spec.version.to_s == "7.2.4" &&
       original_digest == "7570a7fdfffcd3f404dc39c32e84d127c80f40f1be8623aa56767f350557d45b" &&
       backport_digest == "421a529c08bde4d90087f2752260b6fe751ee815887026b57765ff6398912974"
  abort "Active Support changed. Recheck the GHSA-8474-xrp6-wh69 build backport before running CocoaPods."
end

require backport
ActiveSupport::Inflector.prepend(Nu54ActiveSupportLiteralMatch)
unless ActiveSupport::Inflector.method(:safe_constantize).owner == Nu54ActiveSupportLiteralMatch
  abort "Active Support build backport was not selected"
end
warn "Active Support #{spec.version} build backport GHSA-8474-xrp6-wh69 original=#{original_digest} patch=#{backport_digest}"

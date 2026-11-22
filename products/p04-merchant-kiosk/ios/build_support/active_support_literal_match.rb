# frozen_string_literal: true

# Rails 210ac22d446c8e2625b091e79256806b836eee20, for CocoaPods' Active Support 7.2 line.
module Nu54ActiveSupportLiteralMatch
  def safe_constantize(camel_cased_word)
    constantize(camel_cased_word)
  rescue NameError => error
    name = error.name.to_s
    raise if error.name && !camel_cased_word.to_s.split("::").include?(name) && name != camel_cased_word.to_s
  rescue LoadError => error
    message = error.respond_to?(:original_message) ? error.original_message : error.message
    constant_name = camel_cased_word.split("::").find { |part| !part.empty? } || camel_cased_word
    raise unless message.include?("Unable to autoload constant #{constant_name}")
  end
end

# frozen_string_literal: true

require "minitest/autorun"
require_relative "active_support_backport"

class ActiveSupportBuildBackportTest < Minitest::Test
  def test_matching_autoload_message_remains_a_missing_constant
    error = LoadError.new("Unable to autoload constant Nu54Missing")
    ActiveSupport::Inflector.stub(:constantize, ->(_) { raise error }) do
      assert_nil ActiveSupport::Inflector.safe_constantize("::Nu54Missing::Child")
    end
  end

  def test_unrelated_load_failure_is_preserved
    error = LoadError.new("A different dependency could not be loaded")
    ActiveSupport::Inflector.stub(:constantize, ->(_) { raise error }) do
      assert_same error, assert_raises(LoadError) { ActiveSupport::Inflector.safe_constantize("Nu54Missing") }
    end
  end

  def test_runtime_method_is_the_pinned_backport
    assert_equal Nu54ActiveSupportLiteralMatch, ActiveSupport::Inflector.method(:safe_constantize).owner
    assert_equal String, ActiveSupport::Inflector.safe_constantize("String")
    assert_nil ActiveSupport::Inflector.safe_constantize("Nu54UnknownConstant")
  end
end

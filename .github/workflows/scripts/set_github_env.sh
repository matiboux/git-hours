# Helper to set GitHub Actions environment variables

set_github_env() {
	# Inputs (environment variables):
	# - GITHUB_ENV (from GitHub Actions)

	# Inputs (arguments):
	# - var_name: The name of the environment variable to set
	# - var_value: The value of the environment variable to set
	# - is_secret: Whether the value should be masked as secret (true/false) (default: false)
	# - verbose: Whether to output verbose information (true/false) (default: true)

	local var_name="$1"
	local var_value="$2"
	local is_secret="$3"
	local verbose="$4"

	# Verify var_name is not empty
	if [ -z "${var_name}" ]; then
		echo "Error: Variable name is empty" >&2
		return 1
	fi

	# Mask the variable if it's marked as secret
	if [ "${is_secret}" = "true" ] && [ -n "${var_value}" ]; then
		echo "${var_value}" | sed 's/^ */::add-mask::/'
	fi

	# Verbose output
	if [ "${verbose}" != "false" ]; then
		if [ "${is_secret}" = "true" ]; then
			echo "Set ${var_name} (secret)"
		else
			echo "Set ${var_name} to '${var_value}'"
		fi
	fi

	# Export the GitHub Actions environment variable
	echo "${var_name}<<GITHUB_ENV_EOF" >> "$GITHUB_ENV"
	echo "${var_value}" >> "$GITHUB_ENV"
	echo 'GITHUB_ENV_EOF' >> "$GITHUB_ENV"
}

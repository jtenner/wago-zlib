(module
  (type $bytes (array (mut i8)))

  (import "wago_zlib.gc" "compress"
    (func $compress
      (param (ref null $bytes) i32 i32 (ref null $bytes) i32 i32 i32)
      (result i32 i32)))
  (import "wago_zlib.gc" "decompress"
    (func $decompress
      (param (ref null $bytes) i32 i32 (ref null $bytes) i32 i32)
      (result i32 i32)))
  (import "wago_zlib.gc" "compress_packed"
    (func $compress_packed
      (param (ref null $bytes) i32 i32 (ref null $bytes) i32 i32 i32)
      (result i64)))
  (import "wago_zlib.gc" "decompress_packed"
    (func $decompress_packed
      (param (ref null $bytes) i32 i32 (ref null $bytes) i32 i32)
      (result i64)))

  (func (export "success_parity") (result i32)
    (local $src (ref $bytes))
    (local $legacy_dst (ref $bytes))
    (local $packed_dst (ref $bytes))
    (local $legacy_out (ref $bytes))
    (local $packed_out (ref $bytes))
    (local $legacy_status i32)
    (local $legacy_written i32)
    (local $packed i64)
    (local $decode_status i32)
    (local $decode_written i32)
    (local $packed_decode i64)

    (local.set $src
      (array.new_fixed $bytes 3 (i32.const 1) (i32.const 2) (i32.const 3)))
    (local.set $legacy_dst (array.new_default $bytes (i32.const 64)))
    (local.set $packed_dst (array.new_default $bytes (i32.const 64)))
    (local.set $legacy_out (array.new_default $bytes (i32.const 3)))
    (local.set $packed_out (array.new_default $bytes (i32.const 3)))

    (call $compress
      (local.get $src) (i32.const 0) (i32.const 3)
      (local.get $legacy_dst) (i32.const 0) (i32.const 64)
      (i32.const -1))
    (local.set $legacy_written)
    (local.set $legacy_status)
    (local.set $packed
      (call $compress_packed
        (local.get $src) (i32.const 0) (i32.const 3)
        (local.get $packed_dst) (i32.const 0) (i32.const 64)
        (i32.const -1)))

    (call $decompress
      (local.get $legacy_dst) (i32.const 0) (local.get $legacy_written)
      (local.get $legacy_out) (i32.const 0) (i32.const 3))
    (local.set $decode_written)
    (local.set $decode_status)
    (local.set $packed_decode
      (call $decompress_packed
        (local.get $packed_dst) (i32.const 0)
        (i32.wrap_i64 (i64.shr_u (local.get $packed) (i64.const 32)))
        (local.get $packed_out) (i32.const 0) (i32.const 3)))

    (i32.and
      (i32.and
        (i32.eq (local.get $legacy_status) (i32.wrap_i64 (local.get $packed)))
        (i32.eq (local.get $legacy_written)
          (i32.wrap_i64 (i64.shr_u (local.get $packed) (i64.const 32)))))
      (i32.and
        (i32.and
          (i32.eqz (local.get $legacy_status))
          (i32.eq (local.get $decode_written) (i32.const 3)))
        (i32.and
          (i32.and
            (i32.eqz (local.get $decode_status))
            (i32.eq (array.get_s $bytes (local.get $legacy_out) (i32.const 2)) (i32.const 3)))
          (i32.and
            (i32.eqz (i32.wrap_i64 (local.get $packed_decode)))
            (i32.and
              (i32.eq (i32.wrap_i64 (i64.shr_u (local.get $packed_decode) (i64.const 32))) (i32.const 3))
              (i32.eq (array.get_s $bytes (local.get $packed_out) (i32.const 2)) (i32.const 3))))))))

  (func (export "failure_parity") (result i32)
    (local $src (ref $bytes))
    (local $legacy_dst (ref $bytes))
    (local $packed_dst (ref $bytes))
    (local $legacy_status i32)
    (local $legacy_written i32)
    (local $packed i64)

    (local.set $src (array.new_default $bytes (i32.const 0)))
    (local.set $legacy_dst (array.new $bytes (i32.const 165) (i32.const 1)))
    (local.set $packed_dst (array.new $bytes (i32.const 165) (i32.const 1)))
    (call $compress
      (local.get $src) (i32.const 0) (i32.const 0)
      (local.get $legacy_dst) (i32.const 0) (i32.const 1)
      (i32.const 99))
    (local.set $legacy_written)
    (local.set $legacy_status)
    (local.set $packed
      (call $compress_packed
        (local.get $src) (i32.const 0) (i32.const 0)
        (local.get $packed_dst) (i32.const 0) (i32.const 1)
        (i32.const 99)))

    (i32.and
      (i32.and
        (i32.and
          (i32.ne (local.get $legacy_status) (i32.const 0))
          (i32.eqz (local.get $legacy_written)))
        (i32.and
          (i32.eq (local.get $legacy_status) (i32.wrap_i64 (local.get $packed)))
          (i64.eqz (i64.shr_u (local.get $packed) (i64.const 32)))))
      (i32.and
        (i32.eq (array.get_u $bytes (local.get $legacy_dst) (i32.const 0)) (i32.const 165))
        (i32.eq (array.get_u $bytes (local.get $packed_dst) (i32.const 0)) (i32.const 165)))))
)
